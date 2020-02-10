package main

import (
  "os"
  "io"
  "log"
  "time"
  "bytes"
  "strings"
  "io/ioutil"
  "html/template"
  "path"
  "path/filepath"
  "github.com/tdewolff/minify/v2"
  "github.com/tdewolff/minify/v2/js"
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

// File copies a single file from src to dst
func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); check(err)
	defer srcfd.Close()
	if dstfd, err = os.Create(dst); check(err)
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); check(err)
	if srcinfo, err = os.Stat(src); check(err)
	return os.Chmod(dst, srcinfo.Mode())
}

// Dir copies a whole directory recursively
func copyDirectory(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); check(err)
	if err = os.MkdirAll(dst, srcinfo.Mode()); check(err)
	if fds, err = ioutil.ReadDir(src); check(err)

	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcfp, dstfp); check(err)
		} else {
			if err = copyFile(srcfp, dstfp); check(err)
		}
	}

	return nil
}

func copyStaticFiles(src string, dst string) error {
  log.Println("Copying static files...");
  return copyDir(src, dst);
}

func buildHTMLScreens() error {
  log.Println("Building html from markdown...");
  layout, err := ioutil.ReadFile(filepath.Join("./html/", "./layout.html"))
  check(err)

  m := minify.New()
  m.AddFunc("text/css", css.Minify)
  m.AddFunc("text/html", html.Minify)
  m.AddFunc("application/javascript", js.Minify)

  pages := make([]Page, 0)

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

      date, _ := time.Parse("2006-01-02", string(page.Date))
      page.Date = string(date.Format("1 January 2006"))
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

      pages = append(pages, page)
    }

    return err
  })
  check(err)

  // Build index screen
  indexLayoutTemplate, err := template.New("layout").Parse(string(layout))
  check(err)

  indexScreenText, err := ioutil.ReadFile(filepath.Join("./html/", "./index.html"))
  check(err)

  indexScreenTemplate, err := template.New("layout").Parse(string(indexScreenText))
  check(err)

  var fli bytes.Buffer
  err = indexScreenTemplate.Execute(&fli, pages)

  ires, err := m.Bytes("text/html", fli.Bytes())
  check(err)

  var fl bytes.Buffer
  err = indexLayoutTemplate.Execute(&fl, Layout{Content: template.HTML(string(ires)), Title: "Aleksandr Yakunichev", Description: "Aleksandr Yakunichev"})
  check(err)

  result, err := m.Bytes("text/html", fl.Bytes())
  check(err)

  err = ioutil.WriteFile(filepath.Join("./build", "index.html"), result, 0644)
  check(err)
  return nil
}

func main() {
  defer duration(track("Static site built in"))
  buildHTMLScreens()
  copyStaticFiles("./static", "./build")
}
