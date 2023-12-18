package templates

import (
	"embed"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/salsadigitalauorg/rockpool/pkg/platform"

	log "github.com/sirupsen/logrus"
)

//go:embed *.yaml *.tmpl
var templates embed.FS

// Render executes a given template file and returns the path to its
// rendered version.
func Render(tmplName string, values interface{}, destName string) (string, error) {
	log.WithFields(log.Fields{
		"template": tmplName,
		"values":   values,
		"destName": destName,
	}).Debug()
	t := template.Must(template.ParseFS(templates, tmplName))

	var rendered string
	path := RenderedPath(true)
	if tmplName == "registries.yaml" {
		path = RenderedPath(false)
	}
	if destName != "" {
		rendered = filepath.Join(path, destName)
	} else if filepath.Ext(tmplName) == ".tmpl" {
		rendered = filepath.Join(path, strings.TrimSuffix(tmplName, ".tmpl"))
	} else {
		rendered = filepath.Join(path, tmplName)
	}

	f, err := os.Create(rendered)
	if err != nil {
		return "", err
	}

	err = t.Execute(f, values)
	f.Close()
	if err != nil {
		return "", err
	}
	log.WithFields(log.Fields{
		"template": tmplName,
		"rendered": rendered,
	}).Debug("rendered template")
	return rendered, nil
}

func RenderedPath(withName bool) string {
	p := path.Join(platform.ConfigDir, "rendered")
	if withName {
		p = path.Join(p, platform.Name)
	}
	return p
}
