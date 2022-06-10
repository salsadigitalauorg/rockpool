package rockpool

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// Render executes a given template file and returns the path to its
// rendered version.
func (ts *Templates) Render(tn string, config interface{}, destName string) (string, error) {
	t := template.Must(template.ParseFS(templates, "templates/"+tn))

	var rendered string
	path := ts.RenderedPath(true)
	if tn == "registries.yaml" {
		path = ts.RenderedPath(false)
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

func (ts *Templates) RenderedPath(withName bool) string {
	p := path.Join(ts.Config.ConfigDir, "rendered")
	if withName {
		p = path.Join(p, ts.Config.Name)
	}
	return p
}
