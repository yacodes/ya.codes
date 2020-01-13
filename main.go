package main

import (
  "os"
  "bytes"
  "strings"
  "io/ioutil"
  "html/template"
  "path/filepath"
  "gopkg.in/yaml.v2"
  "github.com/tdewolff/minify/v2"
  "github.com/tdewolff/minify/v2/css"
  "github.com/tdewolff/minify/v2/html"
)

type Page struct {
  Title string
  Slug string
  Date string
  Kind string
}

type Layout struct {
  Content template.HTML
  Title string
  Description string
}

func check(e error) {
  if e != nil {
    panic(e)
  }
}

func main() {
  layout, err := ioutil.ReadFile("./layout.html")
  check(err)

  m := minify.New()
  m.AddFunc("text/css", css.Minify)
  m.AddFunc("text/html", html.Minify)

  err = filepath.Walk("./e", func(path string, f os.FileInfo, err error) error {
    if f.IsDir() && strings.HasSuffix(path, ".page") {
      name := f.Name()[11:(len(f.Name()) - 5)]
      // @TODO Use path/filepath
      // @TODO Move to func
      config, err := ioutil.ReadFile(path + "/" + name + ".yaml")
      check(err)
      page := Page{}
      err = yaml.Unmarshal([]byte(config), &page)
      check(err)
      _, err = yaml.Marshal(&page)
      check(err)

      // @TODO Use path/filepath
      tmpl, err := template.ParseFiles(path + "/" + name + ".html")
      check(err)
      var tpl bytes.Buffer
      err = tmpl.Execute(&tpl, page)
      check(err)

      // Wrap with layout
      latmpl, err := template.New("layout").Parse(string(layout))
      check(err)
      var fl bytes.Buffer
      err = latmpl.Execute(&fl, Layout{Content: template.HTML(tpl.String()), Title: page.Title, Description: "test"})

      result, err := m.Bytes("text/html", fl.Bytes())
      check(err)

      err = os.MkdirAll(filepath.Join("./build", "e"), os.ModePerm)
      check(err)

      err = ioutil.WriteFile(filepath.Join("./build", "e/" + page.Slug + ".html"), result, 0644)
      check(err)
    }

    return err
  })
  check(err)
}
