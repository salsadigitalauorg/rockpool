package templates

import (
	"embed"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/salsadigitalauorg/rockpool/pkg/platform"
)

//go:embed *.yaml *.tmpl
var templates embed.FS

// Render executes a given template file and returns the path to its
// rendered version.
func Render(tn string, config interface{}, destName string) (string, error) {
	t := template.Must(template.ParseFS(templates, tn))

	var rendered string
	path := RenderedPath(true)
	if tn == "registries.yaml" {
		path = RenderedPath(false)
	}
	if destName != "" {
		rendered = filepath.Join(path, destName)
	} else if filepath.Ext(tn) == ".tmpl" {
		rendered = filepath.Join(path, strings.TrimSuffix(tn, ".tmpl"))
	} else {
		rendered = filepath.Join(path, tn)
	}

	f, err := os.Create(rendered)
	if err != nil {
		return "", err
	}

	err = t.Execute(f, config)
	f.Close()
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func RenderedPath(withName bool) string {
	p := path.Join(platform.ConfigDir, "rendered")
	if withName {
		p = path.Join(p, platform.Name)
	}
	return p
}
