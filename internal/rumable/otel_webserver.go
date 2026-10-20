package rumable

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func (c *attachConfig) checkApacheOTel(root string, s Site) (bool, error) {
	owner, err := otelOwner(root, s)
	if err != nil {
		return false, err
	}
	ownerSite := s
	if owner != s.ID {
		st, err := loadState(root, owner)
		if err != nil {
			return false, err
		}
		ownerSite = st.Current.Site
	}
	block := insertBlock(ownerSite, "otel-module", "LoadModule otel_apache_module "+apacheQuote(s.OTel.ModulePath)+"\nApacheModuleEnabled OFF\n")
	backup, _ := loadAttachment(root, ownerSite)
	ownedModuleNode := func(n *attachNode) bool {
		if backup.Phase != "attached" && backup.Phase != "applying" {
			return false
		}
		for _, f := range backup.Files {
			live := c.files[n.file]
			start := bytes.Index(live, []byte(block))
			if f.Path == n.file && bytes.Count(f.After, []byte(block)) == 1 && bytes.Count(live, []byte(block)) == 1 && n.start >= start && n.stop <= start+len(block) {
				return true
			}
		}
		return false
	}
	loaded := false
	err = visitNodes(c.root, func(n *attachNode) error {
		if strings.EqualFold(n.words[0], "LoadModule") && len(n.words) > 2 && n.words[1] == "otel_apache_module" {
			if filepath.Clean(n.words[2]) != filepath.Clean(s.OTel.ModulePath) {
				return fmt.Errorf("another Apache OTel module is loaded at %s", n.key())
			}
			if !ownedModuleNode(n) {
				return fmt.Errorf("unmanaged Apache OTel module at %s; automatic attachment must own its global OFF baseline", n.key())
			}
			loaded = true
		}
		if strings.HasPrefix(strings.ToLower(n.words[0]), "apachemodule") && !managedOTelFile(root, n.file) && !ownedModuleNode(n) {
			return fmt.Errorf("unmanaged Apache OTel directive at %s", n.key())
		}
		return nil
	})
	return loaded, err
}

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

func apacheOTel(s Site, active bool) string {
	o := s.OTel
	if o == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ApacheModuleOtelSpanExporter otlp\nApacheModuleOtelExporterEndpoint %s\nApacheModuleOtelSpanProcessor batch\nApacheModuleOtelScheduledDelayMillis 1000\nApacheModuleServiceName %s\nApacheModuleServiceNamespace rumable\nApacheModuleServiceInstanceId %s\n", apacheQuote(strings.TrimPrefix(strings.TrimPrefix(o.nativeEndpoint(s), "http://"), "https://")), apacheQuote(o.ServiceName), apacheQuote(o.ServiceName))
	if strings.HasPrefix(o.nativeEndpoint(s), "https://") {
		b.WriteString("ApacheModuleOtelSslEnabled ON\n")
		if o.TrustedCA != "" {
			fmt.Fprintf(&b, "ApacheModuleOtelSslCertificatePath %s\n", apacheQuote(o.TrustedCA))
		}
	}
	inc, exc := otelPaths(s)
	for i := range inc {
		inc[i] = regexp.QuoteMeta(inc[i])
	}
	for i := range exc {
		exc[i] = regexp.QuoteMeta(exc[i])
	}
	common := b.String()
	b.Reset()
	b.WriteString("<Location \"/\">\n    ApacheModuleEnabled OFF\n</Location>\n")
	if active && o.rate() > 0 {
		fmt.Fprintf(&b, "<LocationMatch %s>\n%s    ApacheModuleEnabled ON\n    ApacheModuleWebserverContext %s rumable %s\n</LocationMatch>\n", apacheQuote("^(?!(?:"+strings.Join(exc, "|")+"))(?:"+strings.Join(inc, "|")+")"), common, apacheQuote(o.ServiceName), apacheQuote(s.ID))
	}
	return b.String()
}

func nginxOTelAttachment(c *attachConfig, root string, s Site, selected *attachNode) ([]attachEdit, error) {
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
	main := selected
	for main.parent != c.root {
		main = main.parent
	}
	return []attachEdit{{main.file, 0, 0, insertBlock(s, "otel-module", "load_module "+nginxQuote(s.OTel.ModulePath)+";\n")}}, nil
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
	if !contains([]string{"http.conf", "location.conf", "server.conf", "apache.conf"}, parts[1]) {
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
