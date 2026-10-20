package rumable

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func otelPaths(s Site) (include, exclude []string) {
	include = append([]string{}, s.OTel.Include...)
	if len(include) == 0 {
		include = []string{"/"}
	}
	exclude = append([]string{strings.TrimSuffix(s.assetPrefix(), s.ID+"/")}, s.OTel.Exclude...)
	return
}

func renderNginxOTel(n *Snapshot) {
	s, o := n.Site, n.Site.OTel
	if o == nil {
		return
	}
	v := "rumable_" + strings.ReplaceAll(s.ID, "-", "_") + "_otel"
	var h strings.Builder
	if n.OTelOwner == s.ID {
		// Module 0.1.0 passes this directly to gRPC's target resolver. A plain
		// host:port works on both old modules and 0.1.2's URL-aware exporter.
		fmt.Fprintf(&h, "otel_exporter {\n    endpoint %s;\n    interval 1s;\n", nginxQuote(strings.TrimPrefix(o.nativeEndpoint(s), "http://")))
		if o.TrustedCA != "" {
			fmt.Fprintf(&h, "    trusted_certificate %s;\n", nginxQuote(o.TrustedCA))
		}
		fmt.Fprintf(&h, "}\notel_service_name %s;\n", nginxQuote(o.ServiceName))
	}
	inc, exc := otelPaths(s)
	fmt.Fprintf(&h, "map $uri $%s_path {\n    default 0;\n", v)
	for _, p := range exc {
		fmt.Fprintf(&h, "    %s 0;\n", nginxQuote("~^"+regexp.QuoteMeta(p)))
	}
	for _, p := range inc {
		fmt.Fprintf(&h, "    %s 1;\n", nginxQuote("~^"+regexp.QuoteMeta(p)))
	}
	h.WriteString("}\n")
	if o.rate() > 0 && o.rate() < 100 {
		fmt.Fprintf(&h, "split_clients $otel_trace_id $%s_sample {\n    %.2f%% on;\n    * off;\n}\n", v, o.rate())
	} else {
		value := "off"
		if o.rate() == 100 {
			value = "on"
		}
		fmt.Fprintf(&h, "map $uri $%s_sample { default %s; }\n", v, value)
	}
	fmt.Fprintf(&h, "map \"$%s_path:$%s_sample\" $%s_selected {\n    default off;\n    \"1:on\" on;\n}\n", v, v, v)
	n.Files["http.conf"] = append(n.Files["http.conf"], h.String()...)
	trace := "otel_trace off;\notel_trace_context ignore;\n"
	if n.OTelEnabled {
		trace = fmt.Sprintf("otel_trace $%s_selected;\notel_trace_context propagate;\notel_span_attr rumable.site.id %s;\n", v, nginxQuote(s.ID))
	}
	n.Files["location.conf"] = append(n.Files["location.conf"], trace...)
	// The asset handler must override server-level OTel inheritance too.
	n.Files["server.conf"] = []byte(strings.Replace(string(n.Files["server.conf"]), "    alias ", "    otel_trace off;\n    otel_trace_context ignore;\n    alias ", 1))
	n.Files["server.conf"] = append(n.Files["server.conf"], fmt.Sprintf("location = %s {\n    otel_trace off;\n    otel_trace_context ignore;\n    default_type text/plain;\n    expires -1;\n    return 200 %s;\n}\n", nginxQuote(s.assetPrefix()+"otel-status.json"), nginxQuote(n.otelGeneration()))...)
}

func nginxOTelAttachment(c *attachConfig, root string, s Site) ([]attachEdit, error) {
	if s.OTel == nil {
		return nil, nil
	}
	_, err := otelOwner(root, s)
	if err != nil {
		return nil, err
	}
	var loaded bool
	err = visitNodes(c.root, func(n *attachNode) error {
		if n.words[0] == "load_module" && len(n.words) == 2 && strings.Contains(n.words[1], "otel") {
			if filepath.Clean(n.words[1]) != filepath.Clean(s.OTel.ModulePath) {
				return fmt.Errorf("another OTel module is loaded at %s", n.key())
			}
			loaded = true
		}
		if strings.HasPrefix(n.words[0], "otel_") {
			managed := managedOTelFile(root, n.file)
			if !managed {
				return fmt.Errorf("existing OTel directive at %s; refusing to take over unmanaged instrumentation", n.key())
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if loaded {
		return nil, nil
	}
	// A main-context include can appear after events/http. Dynamic modules must
	// load before those blocks, at the start of the actual entry configuration.
	return []attachEdit{{c.root.file, 0, 0, insertBlock(s, "otel-module", "load_module "+nginxQuote(s.OTel.ModulePath)+";\n")}}, nil
}

func checkNginxOTel(root string, s Site, version, dump []byte) error {
	if s.OTel == nil {
		return nil
	}
	m := nginxVersion.FindStringSubmatch(string(version))
	if len(m) != 4 || !nginxOTelVersion(m[1], m[2], m[3]) {
		return fmt.Errorf("Nginx OTel requires Nginx >=1.25.3 and a compatible ngx_otel_module; RUM-only support remains >=1.9.4")
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return err
	}
	owner, err := otelOwner(root, s)
	if err != nil {
		return err
	}
	count := 0
	returnErr := visitNodes(c.root, func(n *attachNode) error {
		if !strings.HasPrefix(n.words[0], "otel_") {
			return nil
		}
		if !managedOTelFile(root, n.file) {
			return fmt.Errorf("unmanaged OTel directive at %s", n.key())
		}
		if n.words[0] == "otel_exporter" {
			count++
			want := filepath.Join(root, "sites", owner, "http.conf")
			if filepath.Clean(n.file) != filepath.Clean(want) {
				return fmt.Errorf("unexpected OTel exporter owner at %s", n.key())
			}
		}
		return nil
	})
	if returnErr != nil {
		return returnErr
	}
	if count != 1 {
		return fmt.Errorf("expected one managed OTel exporter, found %d; disable and reattach with OTel configured", count)
	}
	return nil
}

func managedOTelFile(root, file string) bool {
	rel, err := filepath.Rel(filepath.Join(root, "sites"), file)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 2 || !idPattern.MatchString(parts[0]) {
		return false
	}
	if !contains([]string{"http.conf", "location.conf", "server.conf"}, parts[1]) {
		return false
	}
	st, err := loadState(root, parts[0])
	return err == nil && st.Current.Site.OTel != nil
}

func nginxOTelVersion(major, minor, patch string) bool {
	// Numeric comparison avoids lexicographic mistakes such as 1.9 > 1.25.
	var a, b, c int
	_, err := fmt.Sscanf(major+"."+minor+"."+patch, "%d.%d.%d", &a, &b, &c)
	return err == nil && (a > 1 || a == 1 && (b > 25 || b == 25 && c >= 3))
}
