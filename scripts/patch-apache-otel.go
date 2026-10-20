//go:build ignore

// Apply the request isolation guard to the pinned webserver/v1.0.4 source.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run scripts/patch-apache-otel.go PATH/ApacheHooks.cpp")
	}
	name := os.Args[1]
	b, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	s := string(b)
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "e90c11d6a85a0277c49ba7e939a3c3f67a934b1b26134c9ca25f21b5397aa199" {
		panic("ApacheHooks.cpp does not match pinned webserver/v1.0.4 source")
	}
	const anchor = "int ApacheHooks::otel_hook_header_parser_begin(request_rec *r)\n{\n"
	const guard = `    // Process SDK initialization must not bypass per-request site/path opt-out.
    otel_cfg* request_config = ApacheConfigHandlers::getConfig(r);
    if (!request_config || !request_config->getOtelEnabled()) {
        return DECLINED;
    }
`
	if strings.Count(s, anchor) != 1 || strings.Contains(s, "rumable-apache-isolation-v1") {
		panic("unsupported or already patched Apache OTel source; expected original webserver/v1.0.4")
	}
	configName := filepath.Join(filepath.Dir(name), "ApacheConfig.cpp")
	config, err := os.ReadFile(configName)
	if err != nil {
		panic(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(config)) != "c2d8d9fa115b97c801ffab63e56db42885f280403c7b9760b6d830638c08ceb4" {
		panic("ApacheConfig.cpp does not match pinned webserver/v1.0.4 source")
	}
	const check = "if (cfg && cfg->validate(r))"
	if strings.Count(string(config), check) != 1 {
		panic("unsupported ApacheConfig.cpp source")
	}
	config = []byte(strings.Replace(string(config), check, "if (cfg && cfg->getOtelEnabled() && cfg->validate(r))", 1))
	s = strings.Replace(s, anchor, anchor+guard, 1)
	s += "\n" + `extern "C" __attribute__((visibility("default"))) const char* rumable_otel_isolation_v1() {
    return "rumable-apache-isolation-v1";
}
`
	if err = os.WriteFile(name, []byte(s), 0644); err != nil {
		panic(err)
	}
	if err = os.WriteFile(configName, config, 0644); err != nil {
		panic(err)
	}
	fmt.Println("Applied Apache OTel per-request isolation guard")
}
