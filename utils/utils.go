// Package utils provides utility functions.
package utils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/mitchellh/go-homedir"

	"gopkg.in/yaml.v3"
)

func scalarToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", t)
	default:
		// Fallback to YAML-marshaled string for complex types
		b, err := yaml.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return strings.TrimSpace(string(b))
	}
}

// RemoveFrontmatter removes the front matter header of a markdown file.
func RemoveFrontmatter(content []byte) []byte {
	if frontmatterBoundaries := detectFrontmatter(content); frontmatterBoundaries[0] == 0 {
		return content[frontmatterBoundaries[1]:]
	}
	return content
}

// extractFrontmatterVars reads YAML frontmatter (if present) and returns a flattened map plus the bounds.
func extractFrontmatterVars(content []byte) (map[string]string, []int) {
	fmBounds := detectFrontmatter(content)
	vars := make(map[string]string)

	if fmBounds[0] == 0 && fmBounds[1] > fmBounds[0] {
		fmBytes := content[fmBounds[0]:fmBounds[1]]
		// strip the leading and trailing '---' lines
		trim := bytes.TrimPrefix(fmBytes, []byte("---"))
		trim = bytes.TrimSuffix(trim, []byte("---"))
		trim = bytes.TrimSpace(trim)

		var raw map[string]interface{}
		if err := yaml.Unmarshal(trim, &raw); err == nil {
			flattenYAML("", raw, vars)
		}
	}

	return vars, fmBounds
}

func flattenYAML(prefix string, in interface{}, out map[string]string) {
	key := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + "." + k
	}

	switch v := in.(type) {
	case map[string]interface{}:
		for k, vv := range v {
			flattenYAML(key(k), vv, out)
		}
	case []interface{}:
		var parts []string
		for _, item := range v {
			parts = append(parts, scalarToString(item))
		}
		out[prefix] = strings.Join(parts, ", ")
	default:
		out[prefix] = scalarToString(v)
	}
}

// PreprocessDynamicText replaces some contents of the markdown file with dynamically generated contents.
func PreprocessDynamicText(content []byte) []byte {

	vars, _ := extractFrontmatterVars(content)
	content = RemoveFrontmatter(content)

	// Built-ins (non-variable defined vars)
	now := time.Now()
	vars["datetime_rfc3339"] = now.Format(time.RFC3339)
	vars["datetime_rfc1123"] = now.Format(time.RFC1123)
	vars["datetime"] = now.Format("2006-01-02 15:04")
	vars["datetime_iso"] = now.Format("2006-01-02 15:04:05")
	vars["date_short"] = now.Format("2006-01-02")
	vars["date_long"] = now.Format("Jan 02, 2006")
	vars["date_full"] = now.Format("Monday, 02 Jan 2006")
	vars["custom_date"] = now.Format(vars["custom_date_fmt"]) // user custom_date_fmt var to format the date string
	vars["date"] = vars["date_short"]

	vars["time_12h"] = now.Format("03:04 PM")
	vars["time_24h"] = now.Format("15:04")
	vars["time_long"] = now.Format("15:04:05")
	vars["time"] = vars["time_24h"]
	vars["tz_short"] = now.Format("MST")
	vars["tz_offset"] = now.Format("-7:00")
	vars["tz"] = vars["tz_short"]

	cwd, err := os.Getwd()
	if err == nil {
		vars["pwd"] = cwd
		vars["cwd"] = cwd
		cwd_short := filepath.Base(cwd)
		if cwd_short == string(filepath.Separator) || cwd_short == "." {
			cwd_short = cwd // fallback to full path
		}
		vars["pwd_short"] = cwd_short
		vars["cwd_short"] = cwd_short
	}

	for k, v := range vars {
		re := regexp.MustCompile(`\{\{\s*` + regexp.QuoteMeta(k) + `\s*\}\}`)
		content = re.ReplaceAll(content, []byte(v))
	}

	return content
}

var yamlPattern = regexp.MustCompile(`(?m)^---\r?\n(\s*\r?\n)?`)

func detectFrontmatter(c []byte) []int {
	if matches := yamlPattern.FindAllIndex(c, 2); len(matches) > 1 {
		return []int{matches[0][0], matches[1][1]}
	}
	return []int{-1, -1}
}

// ExpandPath expands tilde and all environment variables from the given path.
func ExpandPath(path string) string {
	s, err := homedir.Expand(path)
	if err == nil {
		return os.ExpandEnv(s)
	}
	return os.ExpandEnv(path)
}

// WrapCodeBlock wraps a string in a code block with the given language.
func WrapCodeBlock(s, language string) string {
	return "```" + language + "\n" + s + "```"
}

var markdownExtensions = []string{
	".md", ".mdown", ".mkdn", ".mkd", ".markdown",
}

// IsMarkdownFile returns whether the filename has a markdown extension.
func IsMarkdownFile(filename string) bool {
	ext := filepath.Ext(filename)

	if ext == "" {
		// By default, assume it's a markdown file.
		return true
	}

	for _, v := range markdownExtensions {
		if strings.EqualFold(ext, v) {
			return true
		}
	}

	// Has an extension but not markdown
	// so assume this is a code file.
	return false
}

// GlamourStyle returns a glamour.TermRendererOption based on the given style.
func GlamourStyle(style string, isCode bool) glamour.TermRendererOption {
	if !isCode {
		if style == styles.AutoStyle {
			return glamour.WithAutoStyle()
		}
		return glamour.WithStylePath(style)
	}

	// If we are rendering a pure code block, we need to modify the style to
	// remove the indentation.

	var styleConfig ansi.StyleConfig

	switch style {
	case styles.AutoStyle:
		if lipgloss.HasDarkBackground() {
			styleConfig = styles.DarkStyleConfig
		} else {
			styleConfig = styles.LightStyleConfig
		}
	case styles.DarkStyle:
		styleConfig = styles.DarkStyleConfig
	case styles.LightStyle:
		styleConfig = styles.LightStyleConfig
	case styles.PinkStyle:
		styleConfig = styles.PinkStyleConfig
	case styles.NoTTYStyle:
		styleConfig = styles.NoTTYStyleConfig
	case styles.DraculaStyle:
		styleConfig = styles.DraculaStyleConfig
	case styles.TokyoNightStyle:
		styleConfig = styles.DraculaStyleConfig
	default:
		return glamour.WithStylesFromJSONFile(style)
	}

	var margin uint
	styleConfig.CodeBlock.Margin = &margin

	return glamour.WithStyles(styleConfig)
}
