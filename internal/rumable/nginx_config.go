package rumable

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

type nginxStatement struct {
	words       []string
	end         byte
	line        int
	start, stop int
}

// Only the Nginx directive grammar is needed, not module-specific argument
// semantics. In particular, braces and directive names inside quoted values
// or comments must never change the effective scope.
func nginxStatements(source string) ([]nginxStatement, error) {
	var result []nginxStatement
	var words []string
	var word strings.Builder
	line, start, offset := 1, 1, 0
	var quote byte
	inWord, comment, variable := false, false, false
	finishWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord, variable = false, false
		}
	}
	for i := 0; i < len(source); i++ {
		c := source[i]
		currentLine := line
		if c == '\n' {
			line++
		}
		if comment {
			comment = c != '\n'
			continue
		}
		if !inWord && c == '#' {
			comment = true
			continue
		}
		if c == '\\' {
			if !inWord && len(words) == 0 {
				start = currentLine
				offset = i
			}
			inWord, variable = true, false
			if i+1 == len(source) {
				return nil, fmt.Errorf("line %d: unfinished escape", currentLine)
			}
			i++
			next := source[i]
			switch next {
			case '\\', '\'', '"':
				word.WriteByte(next)
			case 'n':
				word.WriteByte('\n')
			case 'r':
				word.WriteByte('\r')
			case 't':
				word.WriteByte('\t')
			default:
				word.WriteByte('\\')
				word.WriteByte(next)
			}
			if next == '\n' {
				line++
			}
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
				finishWord()
			} else {
				word.WriteByte(c)
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			finishWord()
			continue
		}
		if !inWord && len(words) == 0 {
			start = currentLine
			offset = i
		}
		if !inWord && (c == '\'' || c == '"') {
			quote, inWord = c, true
			continue
		}
		if c == '{' && variable {
			word.WriteByte(c)
			variable = false
			continue
		}
		if c == ';' || c == '{' || (c == '}' && !inWord) {
			finishWord()
			if (c == '}' && len(words) != 0) || (c != '}' && len(words) == 0) {
				return nil, fmt.Errorf("line %d: unexpected configuration delimiter", currentLine)
			}
			result = append(result, nginxStatement{words: words, end: c, line: start, start: offset, stop: i + 1})
			words = nil
			continue
		}
		word.WriteByte(c)
		inWord, variable = true, c == '$'
	}
	if quote != 0 || inWord || len(words) != 0 {
		return nil, fmt.Errorf("line %d: incomplete configuration directive", start)
	}
	return result, nil
}

var nginxDumpHeader = regexp.MustCompile(`(?m)^# configuration file ([^\r\n]+):\r?$`)

func checkNginx194(dump []byte) error {
	source := string(dump)
	headers := nginxDumpHeader.FindAllStringSubmatchIndex(source, -1)
	if len(headers) == 0 || strings.TrimSpace(source[:headers[0][0]]) != "" {
		return fmt.Errorf("cannot inspect Nginx 1.9.4: missing configuration dump")
	}
	files := map[string]string{}
	main := path.Clean(source[headers[0][2]:headers[0][3]])
	for i, header := range headers {
		name := path.Clean(source[header[2]:header[3]])
		end := len(source)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		start := header[1]
		if start < len(source) && source[start] == '\n' {
			start++
		}
		body := source[start:end]
		// 1.9.4 dumps a shared include once per use; newer versions deduplicate.
		if previous, ok := files[name]; ok && strings.TrimSpace(previous) != strings.TrimSpace(body) {
			return fmt.Errorf("cannot inspect Nginx 1.9.4: inconsistent dump for %s", name)
		}
		files[name] = body
	}
	// Relative include paths use the main configuration directory, including
	// includes nested in other directories. They are not relative to each file.
	prefix := path.Dir(main)
	active, reached, checked := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var walk func(string, []string) error
	walk = func(name string, scope []string) error {
		if active[name] || len(active) > 128 || len(scope) > 128 {
			return fmt.Errorf("cannot inspect Nginx 1.9.4: recursive or excessive configuration nesting at %s", name)
		}
		key := name + "\x00" + strings.Join(scope, "\x00")
		if checked[key] {
			return nil
		}
		body, exists := files[name]
		if !exists {
			return fmt.Errorf("cannot inspect Nginx 1.9.4: include missing from dump: %s", name)
		}
		active[name], reached[name] = true, true
		defer delete(active, name)
		statements, err := nginxStatements(body)
		if err != nil {
			return fmt.Errorf("cannot inspect Nginx 1.9.4 configuration %s: %w", name, err)
		}
		base := len(scope)
		scope = append([]string{}, scope...)
		for _, statement := range statements {
			switch statement.end {
			case '{':
				scope = append(scope, statement.words[0])
			case '}':
				if len(scope) == base {
					return fmt.Errorf("cannot inspect Nginx 1.9.4: unmatched block in %s:%d", name, statement.line)
				}
				scope = scope[:len(scope)-1]
			case ';':
				if statement.words[0] == "sub_filter" && len(scope) == 1 && scope[0] == "http" {
					return fmt.Errorf("Nginx 1.9.4 http-level sub_filter is incompatible at %s:%d; move rules to server/location or upgrade to Nginx >=1.9.5", name, statement.line)
				}
				if statement.words[0] == "include" {
					if len(statement.words) != 2 {
						return fmt.Errorf("cannot inspect Nginx 1.9.4: unsupported include at %s:%d", name, statement.line)
					}
					pattern := statement.words[1]
					if !path.IsAbs(pattern) {
						pattern = prefix + "/" + pattern
					}
					includes, err := nginxDumpIncludes(pattern, files)
					if err != nil {
						return fmt.Errorf("cannot inspect Nginx 1.9.4 include at %s:%d: %w", name, statement.line, err)
					}
					for _, include := range includes {
						if err := walk(include, scope); err != nil {
							return err
						}
					}
				}
			}
		}
		if len(scope) != base {
			return fmt.Errorf("cannot inspect Nginx 1.9.4: unclosed block in %s", name)
		}
		checked[key] = true
		return nil
	}
	if err := walk(main, nil); err != nil {
		return err
	}
	for name := range files {
		if !reached[name] {
			return fmt.Errorf("cannot inspect Nginx 1.9.4: unresolved dump context for %s", name)
		}
	}
	return nil
}

func nginxDumpIncludes(pattern string, files map[string]string) ([]string, error) {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{path.Clean(pattern)}, nil
	}
	// Go and POSIX glob differ for bracket/locale classes and wildcard/.. paths.
	// Refuse ambiguous masks instead of silently dropping a risky include.
	if strings.Contains(pattern, "[") {
		return nil, fmt.Errorf("unsupported include mask %q; use explicit paths or * / ? masks", pattern)
	}
	wildcard := false
	for _, part := range strings.Split(pattern, "/") {
		if part == ".." && wildcard {
			return nil, fmt.Errorf("unsupported wildcard/.. include mask %q", pattern)
		}
		wildcard = wildcard || strings.ContainsAny(part, "*?")
	}
	pattern = path.Clean(pattern)
	var result []string
	for name := range files {
		if strings.Contains(pattern, "?") && strings.IndexFunc(name+pattern, func(c rune) bool { return c > 127 }) >= 0 {
			return nil, fmt.Errorf("unsupported non-ASCII ? include mask %q", pattern)
		}
		matches, err := path.Match(pattern, name)
		if err != nil {
			return nil, err
		}
		if matches {
			// POSIX glob does not select dot files with an implicit wildcard.
			parts, names := strings.Split(pattern, "/"), strings.Split(name, "/")
			for i, part := range names {
				if strings.HasPrefix(part, ".") && !strings.HasPrefix(parts[i], ".") && !strings.HasPrefix(parts[i], `\.`) {
					matches = false
				}
			}
		}
		if matches {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}
