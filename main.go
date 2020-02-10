package main

import (
  "os"
  "io"
  "log"
  "sort"
  "time"
  "bytes"
  "io/ioutil"
  "strconv"
  "path"
  "path/filepath"
  "encoding/json"
  "html/template"
  "github.com/tdewolff/minify/v2"
  "github.com/tdewolff/minify/v2/js"
  "github.com/tdewolff/minify/v2/css"
  "github.com/tdewolff/minify/v2/html"
)

// Utils
func duration(msg string, start time.Time) { log.Printf("%v %v\n", msg, time.Since(start)) }
func check(e error) { if e != nil { panic(e) } }

// Program bits
type Config struct {
  Title string `json:"title"`
  Description string `json:"description"`
  Events []Event `json:"events"`
  Collection []Year
}
type Year struct {
  Value int
  Entries []Event
}
type Event struct {
  Title string `json:"title"`
  Date time.Time `json:"date"`
  Slug string `json:"slug"`
  Type []string `json:"type"`
  Filepath string `json:"filepath"`
}
type Layout struct {
  Title string
  Description string
  Content template.HTML
}

func getConfig(fp string) Config {
  byt, err := ioutil.ReadFile(fp); check(err)
  data := Config{}
  err = json.Unmarshal(byt, &data); check(err)

  for i, event := range data.Events {
    efp := path.Base(event.Filepath)
    t, err := time.Parse("2006-01-02", efp[0:10]); check(err)
    data.Events[i].Date = t
    data.Events[i].Slug = efp[11:(len(efp) - 5)]
  }

  collection := make(map[int][]Event)
  for _, event := range data.Events {
    k, err := strconv.Atoi(event.Date.Format("2006")); check(err)
    collection[k] = append(collection[k], event)
  }

  for k, v := range collection {
    data.Collection = append(data.Collection, Year{k, v})
  }

  sort.Slice(data.Collection, func(i, j int) bool {
    return data.Collection[i].Value > data.Collection[j].Value
  })

  return data
}

func executeTemplate(tmpl *template.Template, data interface{}) []byte {
  var byt bytes.Buffer
  err := tmpl.Execute(&byt, data); check(err)
  return byt.Bytes()
}

func minifyHTML(byt []byte) []byte {
  m := minify.New()
  m.AddFunc("text/css", css.Minify)
  m.AddFunc("text/html", html.Minify)
  m.AddFunc("application/javascript", js.Minify)
  result, err := m.Bytes("text/html", byt); check(err)
  return result
}

func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

  srcfd, err = os.Open(src); check(err)
	defer srcfd.Close()
  dstfd, err = os.Create(dst); check(err)
	defer dstfd.Close()

  _, err = io.Copy(dstfd, srcfd); check(err)
  srcinfo, err = os.Stat(src); check(err)
	return os.Chmod(dst, srcinfo.Mode())
}

func copyDirectory(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

  srcinfo, err = os.Stat(src); check(err)
  err = os.MkdirAll(dst, srcinfo.Mode()); check(err)
  fds, err = ioutil.ReadDir(src); check(err)

	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = copyDirectory(srcfp, dstfp); check(err)
		} else {
			err = copyFile(srcfp, dstfp); check(err)
		}
	}

	return nil
}

func main() {
  defer duration("Static site built in", time.Now())

  // 1. Get json config data
  config := getConfig(filepath.Join("./data", "./index.json"))
  // 2. Get layout template
  layout := template.Must(template.ParseFiles(filepath.Join("./data", "./html/layout.html")))

  // 3. Generate index screen
  index := minifyHTML(executeTemplate(
    layout,
    Layout{
      Title: config.Title,
      Description: config.Description,
      Content: template.HTML(executeTemplate(template.Must(template.ParseFiles(filepath.Join("./data", "./html/index.html"))), config)),
    },
  ))

  // 4. Create build & build/e dir
  err := os.MkdirAll(filepath.Join("./build", "e"), os.ModePerm); check(err)
  // 5. Write index html to build dir
  err = ioutil.WriteFile(filepath.Join("./build", "./index.html"), index, 0644); check(err)

  // 6. Copy static to build dir
  copyDirectory("./static", "./build")
}
