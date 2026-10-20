package rumable

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func nginxAttachmentConfig(dump []byte) (*attachConfig, error) {
	headers := nginxDumpHeader.FindAllStringSubmatch(string(dump), -1)
	if len(headers) == 0 {
		return nil, fmt.Errorf("missing Nginx configuration dump")
	}
	dumped := map[string]string{}
	for _, h := range headers {
		dumped[filepath.Clean(h[1])] = ""
	}
	c := &attachConfig{files: map[string][]byte{}, uses: map[string]int{}, root: &attachNode{}}
	prefix := filepath.Dir(headers[0][1])
	active := map[string]bool{}
	var parse func(string, *attachNode) error
	parse = func(file string, parent *attachNode) error {
		file, err := c.read(file)
		if err != nil {
			return err
		}
		if active[file] {
			return fmt.Errorf("recursive include %s", file)
		}
		active[file] = true
		defer delete(active, file)
		statements, err := nginxStatements(string(c.files[file]))
		if err != nil {
			return err
		}
		base := parent
		for _, st := range statements {
			if st.end == '}' {
				if parent == base {
					return fmt.Errorf("unbalanced Nginx block in %s", file)
				}
				parent.close = st.start
				parent = parent.parent
				continue
			}
			n := &attachNode{words: st.words, file: file, line: st.line, start: st.start, stop: st.stop, parent: parent}
			if st.words[0] == "include" {
				if len(st.words) != 2 {
					return fmt.Errorf("unsupported include at %s", n.key())
				}
				pattern := st.words[1]
				if !filepath.IsAbs(pattern) {
					pattern = filepath.Join(prefix, pattern)
				}
				includes, err := nginxDumpIncludes(filepath.ToSlash(pattern), dumped)
				if err != nil {
					return err
				}
				for _, include := range includes {
					if err := parse(include, parent); err != nil {
						return err
					}
				}
			} else {
				parent.children = append(parent.children, n)
				if st.end == '{' {
					parent = n
				}
			}
		}
		if parent != base {
			return fmt.Errorf("unclosed Nginx block in %s", file)
		}
		return nil
	}
	if err := parse(headers[0][1], c.root); err != nil {
		return nil, err
	}
	return c, nil
}

func planNginxAttachment(root string, s Site, target string) (attachmentPlan, error) {
	if err := checkOTelModule(s); err != nil {
		return attachmentPlan{}, err
	}
	v, dump, err := nginxInspect(s)
	if err != nil {
		return attachmentPlan{}, err
	}
	if s.OTel != nil {
		m := nginxVersion.FindStringSubmatch(string(v))
		if len(m) != 4 || !nginxOTelVersion(m[1], m[2], m[3]) {
			return attachmentPlan{}, fmt.Errorf("Nginx OTel requires Nginx >=1.25.3 and a compatible ngx_otel_module")
		}
	}
	// The risky 1.9.4 configuration must be rejected before any attachment writes.
	if nginxVersion.FindString(string(v)) == "nginx/1.9.4" {
		if err = checkNginx194(dump); err != nil {
			return attachmentPlan{}, err
		}
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return attachmentPlan{}, err
	}
	files, selected, err := c.planNginx(root, s, target, string(v))
	return c.plan(files, selected), err
}
func (c *attachConfig) planNginx(root string, s Site, target, version string) ([]attachmentFile, string, error) {
	if err := c.checkNamespace(s); err != nil {
		return nil, "", err
	}
	var candidates []*attachNode
	visitNodes(c.root, func(n *attachNode) error {
		if n.words[0] == "server" && n.parent.words != nil && n.parent.words[0] == "http" {
			candidates = append(candidates, n)
		}
		return nil
	})
	names := func(n *attachNode) []string {
		var result []string
		for _, v := range nodeDirect(n, "server_name") {
			result = append(result, v.words[1:]...)
		}
		return result
	}
	selected, err := selectAttachment(s, candidates, target, names)
	if err != nil {
		return nil, "", err
	}
	http := selected.parent
	d := filepath.ToSlash(siteDir(root, s))
	includes := func(name string) string { return "include " + nginxQuote(d+"/"+name) + ";\n" }
	edits, err := nginxOTelAttachment(c, root, s, selected)
	if err != nil {
		return nil, "", err
	}
	edits = append(edits, attachEdit{http.file, http.close, http.close, insertBlock(s, "http", includes("http.conf"))})
	// A competing reserved handler, server rewrite or a non-HTTP upstream cannot
	// be safely merged by inserting a few directives. Keep these on the manual path.
	scopes := []*attachNode{selected}
	err = visitNodes(selected, func(n *attachNode) error {
		if !s.RUM.isEnabled() && n.words[0] != "location" {
			return nil
		}
		switch n.words[0] {
		case "location":
			if strings.Contains(strings.Join(n.words, " "), "__rumable__") {
				return fmt.Errorf("reserved location at %s", n.key())
			}
			scopes = append(scopes, n)
		case "if", "rewrite", "fastcgi_pass", "uwsgi_pass", "scgi_pass", "grpc_pass", "ssi", "addition_types", "add_before_body", "add_after_body":
			return fmt.Errorf("automatic attachment cannot verify %s at %s; use ATTACH.md", n.words[0], n.key())
		case "try_files":
			for _, v := range n.words[1:] {
				if strings.Contains(v, ".gz") || strings.Contains(v, ".br") {
					return fmt.Errorf("precompressed try_files at %s; use ATTACH.md", n.key())
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	// Disabling an inherited body filter would change business responses, including
	// in the initial disabled deployment. Require an explicit manual merge instead.
	for _, scope := range scopes {
		if !s.RUM.isEnabled() {
			body := includes("location.conf")
			if scope == selected {
				body = includes("server.conf") + body
			}
			edits = append(edits, attachEdit{scope.file, scope.close, scope.close, insertBlock(s, "otel-handler-"+hash([]byte(scope.key() + ":" + strconv.Itoa(scope.start)))[:12], body)})
			continue
		}
		for _, directive := range []string{"sub_filter", "sub_filter_once", "sub_filter_last_modified", "etag", "if_modified_since"} {
			if nodes := inherited(scope, directive); len(nodes) > 0 {
				return nil, "", fmt.Errorf("existing %s at %s needs manual merge in ATTACH.md", directive, nodes[0].key())
			}
		}
		body := ""
		if scope == selected {
			body += includes("server.conf")
		}
		body += includes("location.conf")
		for _, module := range []struct{ flag, directive string }{{"--with-http_gzip_static_module", "gzip_static"}, {"ngx_brotli", "brotli_static"}} {
			if strings.Contains(version, module.flag) || len(inherited(scope, module.directive)) > 0 {
				local := nodeDirect(scope, module.directive)
				if len(local) > 0 {
					for _, n := range local {
						edits = append(edits, attachEdit{n.file, n.start, n.stop, module.directive + " off;"})
					}
				} else {
					body += module.directive + " off;\n"
				}
			}
		}
		if len(inherited(scope, "proxy_pass")) > 0 {
			headers := inherited(scope, "proxy_set_header")
			local := nodeDirect(scope, "proxy_set_header")
			critical := map[string]bool{"accept-encoding": true, "if-none-match": true, "if-modified-since": true}
			if len(local) == 0 {
				for _, n := range headers {
					if len(n.words) > 1 && !critical[strings.ToLower(n.words[1])] {
						body += string(c.files[n.file][n.start:n.stop]) + "\n"
					}
				}
			}
			for _, n := range local {
				if len(n.words) > 1 && critical[strings.ToLower(n.words[1])] {
					edits = append(edits, attachEdit{n.file, n.start, n.stop, ""})
				}
			}
			body += "proxy_set_header Accept-Encoding \"\";\nproxy_set_header If-None-Match \"\";\nproxy_set_header If-Modified-Since \"\";\n"
			if local := nodeDirect(scope, "proxy_cache"); len(local) > 0 {
				for _, n := range local {
					edits = append(edits, attachEdit{n.file, n.start, n.stop, "proxy_cache off;"})
				}
			} else {
				body += "proxy_cache off;\n"
			}
		}
		edits = append(edits, attachEdit{scope.file, scope.close, scope.close, insertBlock(s, "handler-"+hash([]byte(scope.key() + ":" + strconv.Itoa(scope.start)))[:12], body)})
	}
	files, err := c.edits(s.ID, edits)
	return files, selected.key(), err
}
