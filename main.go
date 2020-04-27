package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/yuin/goldmark"
	mdhtml "github.com/yuin/goldmark/renderer/html"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"
)

func CopyDirectory(scrDir, dest string) error {
	entries, err := ioutil.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateIfNotExists(destPath, 0755); err != nil {
				return err
			}
			if err := CopyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		case os.ModeSymlink:
			if err := CopySymLink(sourcePath, destPath); err != nil {
				return err
			}
		default:
			if err := Copy(sourcePath, destPath); err != nil {
				return err
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}

		isSymlink := entry.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, entry.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Copy(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	defer out.Close()

	in, err := os.Open(srcFile)
	defer in.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func CopySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}

func logDuration(msg string, start time.Time) {
	fmt.Println(msg, time.Since(start))
}

type Data struct {
	Title       string
	Description string
	Content     template.HTML
}

func transformMarkdownToHTML(event Event, layoutTemplate template.Template, wg *sync.WaitGroup) {
	// Read file
	content, err := ioutil.ReadFile(event.Path)
	if err != nil {
		fmt.Println(err)
	}

	md := goldmark.New(
		goldmark.WithRendererOptions(
			mdhtml.WithUnsafe(),
		),
	)

	// Convert markdown to html
	var buf bytes.Buffer
	if err := md.Convert(content, &buf); err != nil {
		fmt.Println(err)
	}

	var buf2 bytes.Buffer
	eventTemplate := template.Must(template.ParseFiles(filepath.Join(TemplatesDirectory, "./event.tmpl.html")))
	err = eventTemplate.Execute(&buf2, struct{ Content template.HTML }{Content: template.HTML(buf.Bytes())})
	if err != nil {
		panic(err)
	}

	var buf3 bytes.Buffer
	err = layoutTemplate.Execute(&buf3, Data{
		Title:       event.Title,
		Description: event.Description,
		Content:     template.HTML(buf2.Bytes()),
	})
	if err != nil {
		fmt.Println(err)
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/javascript", js.Minify)

	result, err := m.Bytes("text/html", buf3.Bytes())
	if err != nil {
		fmt.Println(err)
	}

	filename := filepath.Join(BuildDirectory, "e", event.Slug+".html")

	err = ioutil.WriteFile(filename, result, 0644)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("✓ File %s is written\n", filename)

	wg.Done()
}

const (
	ConfigPath         = "./index.json"
	StaticDirectory    = "./static"
	BuildDirectory     = "./build"
	EventsDirectory    = "./events"
	TemplatesDirectory = "./templates"
)

var (
	VenueRegexp     = regexp.MustCompile(`\[([^)]+)\]`)
	DelimeterRegexp = regexp.MustCompile(`-`)
)

type Event struct {
	Path        string    `json:"path"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
	Venue       string    `json:"venue"`
}

type Meta struct {
	Title       string
	Description string
}

type Config struct {
	Meta   Meta    `json:"meta"`
	Events []Event `json:"events"`
}

func main() {
	defer logDuration("Process finished in", time.Now())

	// Build directory
	fmt.Println("! Clean up build directory")
	err := os.RemoveAll(BuildDirectory)
	if err != nil {
		fmt.Println(err)
	}
	err = os.MkdirAll(filepath.Join(BuildDirectory, "e"), os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}

	// Read config
	configFile, err := ioutil.ReadFile(ConfigPath)
	if err != nil {
		fmt.Println(err)
	}

	config := Config{}
	err = json.Unmarshal([]byte(configFile), &config)
	if err != nil {
		fmt.Println(err)
	}

	// Layout template
	layoutTemplate := template.Must(template.ParseFiles(filepath.Join(TemplatesDirectory, "./layout.tmpl.html")))

	wg := sync.WaitGroup{}
	for _, e := range config.Events {
		wg.Add(1)
		go transformMarkdownToHTML(e, *layoutTemplate, &wg)
	}
	wg.Wait()

	var buf bytes.Buffer
	indexTemplate := template.Must(template.ParseFiles(filepath.Join(TemplatesDirectory, "./index.tmpl.html")))
	err = indexTemplate.Execute(&buf, struct{ Events []Event }{Events: config.Events})
	if err != nil {
		panic(err)
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/javascript", js.Minify)

	var buf2 bytes.Buffer
	err = layoutTemplate.Execute(&buf2, Data{
		Title:       config.Meta.Title,
		Description: config.Meta.Description,
		Content:     template.HTML(buf.Bytes()),
	})

	result, err := m.Bytes("text/html", buf2.Bytes())
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile(filepath.Join(BuildDirectory, "index.html"), result, 0644)
	if err != nil {
		panic(err)
	}
	fmt.Println("✓ File index.html built")

	// Static directory
	err = CopyDirectory(StaticDirectory, filepath.Join(BuildDirectory))
	if err != nil {
		panic(err)
	}
	fmt.Println("✓ Directory static copied to build/static")
}
