package rumable

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Positions always refer to the original bytes; includes are expanded only in
// memory. A shared include is never edited as if it belonged to one site.
type attachNode struct {
	words                    []string
	file                     string
	line, start, stop, close int
	children                 []*attachNode
	parent                   *attachNode
}
type attachConfig struct {
	files      map[string][]byte
	uses       map[string]int
	root       *attachNode
	aliases    map[string]string
	identities map[string]string
}
type attachmentPlan struct {
	Files      []attachmentFile
	Target     string
	Inputs     map[string]string
	Aliases    map[string]string
	Identities map[string]string
}

func (c *attachConfig) plan(files []attachmentFile, target string) attachmentPlan {
	inputs := map[string]string{}
	for name, b := range c.files {
		inputs[name] = hash(b)
	}
	return attachmentPlan{Files: files, Target: target, Inputs: inputs, Aliases: c.aliases, Identities: c.identities}
}

type attachEdit struct {
	file        string
	start, stop int
	text        string
}

func (c *attachConfig) read(name string) (string, error) {
	alias, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	name, err = filepath.EvalSymlinks(name)
	if err != nil {
		return "", err
	}
	if c.aliases == nil {
		c.aliases = map[string]string{}
	}
	name, err = filepath.Abs(name)
	if err != nil {
		return "", err
	}
	c.aliases[alias] = name
	c.uses[name]++
	if c.uses[name] > 128 {
		return "", fmt.Errorf("recursive or excessive include: %s", name)
	}
	if _, ok := c.files[name]; !ok {
		identity, err := attachmentIdentity(name)
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return "", err
		}
		if len(b) > 8<<20 {
			return "", fmt.Errorf("configuration too large: %s", name)
		}
		after, err := attachmentIdentity(name)
		if err != nil || identity != after {
			return "", fmt.Errorf("configuration changed while reading %s", name)
		}
		if c.identities == nil {
			c.identities = map[string]string{}
		}
		c.identities[name] = identity
		c.files[name] = b
	}
	return name, nil
}
func nodeDirect(n *attachNode, directive string) []*attachNode {
	var result []*attachNode
	for _, v := range n.children {
		if strings.EqualFold(v.words[0], directive) {
			result = append(result, v)
		}
	}
	return result
}
func inherited(n *attachNode, directive string) []*attachNode {
	for ; n != nil; n = n.parent {
		if v := nodeDirect(n, directive); len(v) > 0 {
			return v
		}
	}
	return nil
}
func visitNodes(n *attachNode, f func(*attachNode) error) error {
	for _, v := range n.children {
		if err := f(v); err != nil {
			return err
		}
		if err := visitNodes(v, f); err != nil {
			return err
		}
	}
	return nil
}
func (n *attachNode) key() string { return fmt.Sprintf("%s:%d", n.file, n.line) }
func selectAttachment(s Site, candidates []*attachNode, target string, names func(*attachNode) []string) (*attachNode, error) {
	if target != "" {
		var matches []*attachNode
		for _, n := range candidates {
			if n.key() == target {
				matches = append(matches, n)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("ambiguous target %s: put each server block on a separate line before attaching", target)
		}
	}
	u, _ := url.Parse(s.PublicURL)
	var portMatches []*attachNode
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	for _, n := range candidates {
		for _, name := range names(n) {
			if strings.EqualFold(strings.TrimSuffix(name, "."), strings.TrimSuffix(u.Hostname(), ".")) {
				ports := []string{}
				if strings.EqualFold(n.words[0], "server") {
					listeners := nodeDirect(n, "listen")
					if len(listeners) == 0 {
						ports = append(ports, "80")
					}
					for _, v := range listeners {
						if len(v.words) > 1 {
							ports = append(ports, configPort(v.words[1]))
						}
					}
				} else {
					for _, v := range n.words[1:] {
						ports = append(ports, configPort(v))
					}
				}
				for _, p := range ports {
					if p == port {
						portMatches = append(portMatches, n)
						break
					}
				}
				break
			}
		}
	}
	if target == "" && len(portMatches) == 1 {
		return portMatches[0], nil
	}
	var b strings.Builder
	b.WriteString("cannot select one site; rerun attach with --target FILE:LINE from these candidates:\n")
	for _, n := range candidates {
		fmt.Fprintf(&b, "  %s  %s\n", n.key(), strings.Join(names(n), ", "))
	}
	return nil, fmt.Errorf("%s", b.String())
}

func (c *attachConfig) checkNamespace(s Site) error {
	for name, b := range c.files {
		if strings.Contains(string(b), "# BEGIN rumable-"+s.ID+" ") {
			return fmt.Errorf("site id %s is already attached in %s; use its original state directory", s.ID, name)
		}
	}
	variable := "$rumable_" + strings.ReplaceAll(s.ID, "-", "_") + "_"
	filter := "RUMABLE_" + strings.ReplaceAll(s.ID, "-", "_")
	return visitNodes(c.root, func(n *attachNode) error {
		for _, word := range n.words {
			reservedVariable := false
			for _, suffix := range []string{"path", "html", "download", "tag", "anchor"} {
				if strings.EqualFold(word, variable+suffix) {
					reservedVariable = true
				}
			}
			if reservedVariable || strings.EqualFold(word, filter) || word == s.assetPrefix() {
				return fmt.Errorf("existing Rumable namespace for %s at %s; use its original state directory or another site id", s.ID, n.key())
			}
		}
		return nil
	})
}
func insertBlock(s Site, label, body string) string {
	return "\n# BEGIN rumable-" + s.ID + " " + label + "\n" + body + "# END rumable-" + s.ID + " " + label + "\n"
}
func (c *attachConfig) edits(id string, edits []attachEdit) ([]attachmentFile, error) {
	byFile := map[string][]attachEdit{}
	for _, e := range edits {
		if c.uses[e.file] != 1 {
			return nil, fmt.Errorf("shared include %s is used %d times; use manual ATTACH.md", e.file, c.uses[e.file])
		}
		byFile[e.file] = append(byFile[e.file], e)
	}
	var result []attachmentFile
	var names []string
	for name := range byFile {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		before := c.files[name]
		if strings.Contains(string(before), "# BEGIN rumable-"+id+" ") {
			return nil, fmt.Errorf("existing attachment marker in %s; use its original state directory", name)
		}
		changes := byFile[name]
		sort.SliceStable(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
		var after strings.Builder
		cursor := 0
		var diff strings.Builder
		for _, e := range changes {
			if e.start < cursor || e.stop < e.start {
				return nil, fmt.Errorf("overlapping changes in %s", name)
			}
			after.Write(before[cursor:e.start])
			after.WriteString(e.text)
			cursor = e.stop
			fmt.Fprintf(&diff, "@@ %s:%d @@\n", name, 1+strings.Count(string(before[:e.start]), "\n"))
			for _, entry := range []struct{ prefix, text string }{{"-", string(before[e.start:e.stop])}, {"+", e.text}} {
				for _, line := range strings.Split(strings.TrimSuffix(entry.text, "\n"), "\n") {
					if line != "" {
						fmt.Fprintln(&diff, entry.prefix+line)
					}
				}
			}
		}
		after.Write(before[cursor:])
		info, err := os.Stat(name)
		if err != nil {
			return nil, err
		}
		result = append(result, attachmentFile{Path: name, Before: before, After: []byte(after.String()), Mode: info.Mode().Perm(), Diff: diff.String()})
	}
	return result, nil
}
func configPort(value string) string {
	if i := strings.LastIndex(value, ":"); i >= 0 {
		value = value[i+1:]
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value
	}
	return ""
}
