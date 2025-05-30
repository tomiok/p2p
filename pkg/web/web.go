package web

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"path/filepath"
)

var functions = template.FuncMap{
	"Cut": Trim,
}

func Trim(s string) string {
	if len(s) < 150 {
		return s
	}
	return s[:150] + "..."
}

type TemplateData struct{}

var templates = make(map[string]*template.Template)

func Render(w http.ResponseWriter, tmpl string, td TemplateData, isCached bool) error {
	var (
		t   *template.Template
		err error
		ok  bool
	)

	if isCached {
		if len(templates) == 0 {
			templates, err = templateRenderCache()
			if err != nil {
				return err
			}
		}

		t, ok = templates[tmpl]
		if !ok {
			return errors.New("cache is not available, turn flag to false")
		}
	} else {
		cache, err := templateRenderCache()

		if err != nil {
			return err
		}

		t = cache[tmpl]
	}

	buf := new(bytes.Buffer)
	err = t.Execute(buf, td)

	if err != nil {
		return err
	}

	_, err = buf.WriteTo(w)

	if err != nil {
		return err
	}

	return nil
}

func templateRenderCache() (map[string]*template.Template, error) {
	pages, err := filepath.Glob("./platform/templates/*.page.tmpl")
	var templateCache = make(map[string]*template.Template)

	if err != nil {
		return templateCache, err
	}

	for _, page := range pages {
		name := filepath.Base(page)

		ts, err := template.New(name).Funcs(functions).ParseFiles(page)

		if err != nil {
			return templateCache, err
		}

		matches, err := filepath.Glob("./platform/templates/*.layout.tmpl")

		if err != nil {
			return templateCache, err
		}

		if len(matches) > 0 {
			ts, err = ts.ParseGlob("./platform/templates/*.layout.tmpl")
			if err != nil {
				return templateCache, err
			}
			templateCache[name] = ts
		}
	}

	return templateCache, nil
}
