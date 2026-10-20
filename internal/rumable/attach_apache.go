package rumable

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Apache directives are line based. Preserve byte offsets and continuations;
// native -S, rather than text matching, identifies active VirtualHosts.
func apacheAttachmentTree(file string, b []byte) (*attachNode, error) {
	root := &attachNode{}
	parent := root
	offset, line := 0, 1
	for offset < len(b) {
		start, first := offset, line
		var logical strings.Builder
		for {
			end := strings.IndexByte(string(b[offset:]), '\n')
			if end < 0 {
				end = len(b) - offset
			} else {
				end++
			}
			part := string(b[offset : offset+end])
			offset += end
			line++
			part = strings.TrimRight(part, "\r\n")
			if strings.HasSuffix(part, "\\") {
				logical.WriteString(strings.TrimSuffix(part, "\\"))
				if offset < len(b) {
					continue
				}
			}
			logical.WriteString(part)
			break
		}
		text := strings.TrimSpace(logical.String())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		block := strings.HasPrefix(text, "<")
		if block {
			if !strings.HasSuffix(text, ">") {
				return nil, fmt.Errorf("unsupported block at %s:%d", file, first)
			}
			text = strings.TrimSuffix(strings.TrimPrefix(text, "<"), ">")
		}
		words, err := apacheWords(text)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", file, first, err)
		}
		if len(words) == 0 {
			continue
		}
		if block && strings.HasPrefix(words[0], "/") {
			if parent == root || !strings.EqualFold(strings.TrimPrefix(words[0], "/"), parent.words[0]) {
				return nil, fmt.Errorf("unbalanced Apache block at %s:%d", file, first)
			}
			parent.close = start
			parent = parent.parent
			continue
		}
		n := &attachNode{words: words, file: file, line: first, start: start, stop: offset, parent: parent}
		parent.children = append(parent.children, n)
		if block {
			parent = n
		}
	}
	if parent != root {
		return nil, fmt.Errorf("unclosed Apache block in %s", file)
	}
	return root, nil
}
func apacheWords(text string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote byte
	in := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '\\' && i+1 < len(text) {
			i++
			word.WriteByte(text[i])
			in = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteByte(ch)
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			in = true
			continue
		}
		if ch == ' ' || ch == '\t' {
			if in {
				words = append(words, word.String())
				word.Reset()
				in = false
			}
			continue
		}
		word.WriteByte(ch)
		in = true
	}
	if quote != 0 {
		return nil, fmt.Errorf("unfinished quoted argument")
	}
	if in {
		words = append(words, word.String())
	}
	return words, nil
}

var apacheVhostSource = regexp.MustCompile(`(?m)\(([^()\r\n]+):([0-9]+)\)\s*$`)

func planApacheAttachment(root string, s Site, target string) (attachmentPlan, error) {
	dump, err := apacheInspect(s)
	if err != nil {
		return attachmentPlan{}, err
	}
	x := s.Apache
	active, err := command(x.Binary, append(append([]string{}, x.Args...), "-S")...)
	if err != nil {
		return attachmentPlan{}, err
	}
	c, aliases, err := apacheAttachmentConfig(dump)
	if err != nil {
		return attachmentPlan{}, err
	}
	activeKeys := map[string]bool{}
	for _, m := range apacheVhostSource.FindAllStringSubmatch(string(active), -1) {
		if canonical, ok := aliases[filepath.Clean(m[1])]; ok {
			activeKeys[canonical+":"+m[2]] = true
		}
	}
	files, selected, err := c.planApache(root, s, target, activeKeys)
	return c.plan(files, selected), err
}
func (c *attachConfig) planApache(root string, s Site, target string, active map[string]bool) ([]attachmentFile, string, error) {
	if err := c.checkNamespace(s); err != nil {
		return nil, "", err
	}
	var candidates []*attachNode
	visitNodes(c.root, func(n *attachNode) error {
		if strings.EqualFold(n.words[0], "VirtualHost") && active[n.key()] {
			candidates = append(candidates, n)
		}
		return nil
	})
	names := func(n *attachNode) []string {
		var names []string
		for _, key := range []string{"ServerName", "ServerAlias"} {
			for _, v := range nodeDirect(n, key) {
				for _, name := range v.words[1:] {
					if key == "ServerName" {
						if i := strings.LastIndex(name, ":"); i >= 0 {
							name = name[:i]
						}
					}
					names = append(names, name)
				}
			}
		}
		return names
	}
	selected, err := selectAttachment(s, candidates, target, names)
	if err != nil {
		return nil, "", err
	}
	for parent := selected.parent; parent != nil && parent.words != nil; parent = parent.parent {
		if key := strings.ToLower(parent.words[0]); key != "ifmodule" && key != "ifdefine" {
			return nil, "", fmt.Errorf("VirtualHost in %s needs manual attachment", parent.words[0])
		}
	}
	typed := map[string]bool{}
	visitNodes(c.root, func(n *attachNode) error {
		if apacheApplies(n, selected) && strings.EqualFold(n.words[0], "FilterDeclare") && len(n.words) == 3 && strings.EqualFold(n.words[2], "CONTENT_SET") {
			typed[strings.ToLower(n.words[1])] = true
		}
		return nil
	})
	// Inspect all non-vhost config too: it can be inherited through included
	// Directory/Location or conditional blocks. Unknown output chains need review.
	err = visitNodes(c.root, func(n *attachNode) error {
		if !apacheApplies(n, selected) {
			return nil
		}
		key := strings.ToLower(n.words[0])
		switch key {
		case "rewriteengine":
			if len(n.words) > 1 && strings.EqualFold(n.words[1], "off") {
				return nil
			}
			fallthrough
		case "rewriterule", "setoutputfilter", "addoutputfilter", "addoutputfilterbytype", "filterchain", "filterprovider", "luahookinsertfilter", "luaoutputfilter":
			// Explicit typed compression is safe; other providers may be business
			// filters, but their stage/inheritance needs manual review in this first hook.
			if key == "filterprovider" && len(n.words) > 2 && strings.EqualFold(n.words[2], "DEFLATE") && typed[strings.ToLower(n.words[1])] {
				return nil
			}
			if key == "filterchain" {
				for _, name := range n.words[1:] {
					name = strings.TrimLeft(name, "+=-@")
					if !typed[strings.ToLower(name)] && !strings.EqualFold(name, "DEFLATE") {
						return fmt.Errorf("output chain needs manual review at %s", n.key())
					}
				}
				return nil
			}
			return fmt.Errorf("automatic attachment cannot verify %s at %s; use ATTACH.md", n.words[0], n.key())
		case "filterdeclare":
			if n.parent != c.root && n.parent != selected {
				return fmt.Errorf("conditional/directory output filter needs manual review at %s", n.key())
			}
			if len(n.words) != 3 || !strings.EqualFold(n.words[2], "CONTENT_SET") {
				return fmt.Errorf("output filter stage needs manual review at %s", n.key())
			}
		case "allowoverride":
			if len(n.words) != 2 || !strings.EqualFold(n.words[1], "None") {
				return fmt.Errorf(".htaccess overrides need manual review at %s", n.key())
			}
		case "alias", "location", "locationmatch", "proxypass":
			if strings.Contains(strings.Join(n.words[1:], " "), "__rumable__") {
				return fmt.Errorf("reserved resource mapping at %s", n.key())
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	// Put proxy exclusion before every vhost include/ProxyPass; put the managed
	// LocationMatch last so ordinary preceding sections cannot override it.
	before := insertBlock(s, "proxy", "<IfModule proxy_module>\nProxyPass "+apacheQuote(s.assetPrefix())+" !\n</IfModule>\n")
	after := insertBlock(s, "vhost", "Include "+apacheQuote(filepath.ToSlash(filepath.Join(siteDir(root, s), "apache.conf")))+"\n")
	files, err := c.edits(s.ID, []attachEdit{{selected.file, selected.stop, selected.stop, before}, {selected.file, selected.close, selected.close, after}})
	return files, selected.key(), err
}
func firstWord(n *attachNode) string {
	if len(n.words) > 0 {
		return n.words[0]
	}
	return ""
}

// DUMP_INCLUDES records each include's parent line and recursion indentation.
// Use that native tree rather than treating vhost includes as global config.
var apacheDumpLine = regexp.MustCompile(`^( +)\((\*|[0-9]+)\) (.+)$`)

type apacheDumpFile struct {
	name     string
	includes map[int][]*apacheDumpFile
}

func apacheAttachmentConfig(dump []byte) (*attachConfig, map[string]string, error) {
	c := &attachConfig{files: map[string][]byte{}, uses: map[string]int{}, root: &attachNode{}}
	aliases := map[string]string{}
	var main *apacheDumpFile
	var stack []*apacheDumpFile
	for _, line := range strings.Split(string(dump), "\n") {
		m := apacheDumpLine.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		depth := len(m[1])/2 - 1
		if len(m[1])%2 != 0 || depth < 0 || depth > len(stack) || depth > 128 {
			return nil, nil, fmt.Errorf("unsupported Apache include indentation")
		}
		name, err := c.read(m[3])
		if err != nil {
			return nil, nil, err
		}
		aliases[filepath.Clean(m[3])] = name
		entry := &apacheDumpFile{name: name, includes: map[int][]*apacheDumpFile{}}
		if depth == 0 {
			if main != nil || m[2] != "*" {
				return nil, nil, fmt.Errorf("ambiguous Apache main configuration")
			}
			main = entry
		} else {
			line, err := strconv.Atoi(m[2])
			if err != nil || line < 1 {
				return nil, nil, fmt.Errorf("invalid Apache include line")
			}
			parent := stack[depth-1]
			parent.includes[line] = append(parent.includes[line], entry)
		}
		stack = append(stack[:depth], entry)
	}
	if main == nil {
		return nil, nil, fmt.Errorf("missing Apache DUMP_INCLUDES")
	}
	var expand func(*apacheDumpFile, *attachNode) error
	expand = func(file *apacheDumpFile, parent *attachNode) error {
		tree, err := apacheAttachmentTree(file.name, c.files[file.name])
		if err != nil {
			return err
		}
		used := map[int]bool{}
		var graft func(*attachNode, *attachNode) error
		graft = func(from, to *attachNode) error {
			for _, n := range from.children {
				if strings.EqualFold(n.words[0], "Include") || strings.EqualFold(n.words[0], "IncludeOptional") {
					for _, entry := range file.includes[n.line] {
						if err := expand(entry, to); err != nil {
							return err
						}
					}
					used[n.line] = true
					continue
				}
				children := n.children
				n.children = nil
				n.parent = to
				to.children = append(to.children, n)
				if err := graft(&attachNode{children: children}, n); err != nil {
					return err
				}
			}
			return nil
		}
		if err = graft(tree, parent); err != nil {
			return err
		}
		for line := range file.includes {
			if !used[line] {
				return fmt.Errorf("cannot resolve Apache include context at %s:%d; use ATTACH.md", file.name, line)
			}
		}
		return nil
	}
	if err := expand(main, c.root); err != nil {
		return nil, nil, err
	}
	return c, aliases, nil
}
func apacheApplies(n, target *attachNode) bool {
	for ; n != nil; n = n.parent {
		if strings.EqualFold(firstWord(n), "VirtualHost") {
			return n == target
		}
	}
	return true
}
