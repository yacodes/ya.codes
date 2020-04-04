package main

import (
	"bytes"
	"fmt"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/yuin/goldmark"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func getMarkdownFilenames(path string) []string {
	var filenames []string

	if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".md" {
			filenames = append(filenames, path)
		}
		return nil
	}); err != nil {
		fmt.Println(err)
	}

	return filenames
}

type Data struct {
	Title       string
	Description string
	Content     template.HTML
}

func transformMarkdownToHTML(path string, layoutTemplate template.Template, wg *sync.WaitGroup) {
	// Read file
	content, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println(err)
	}

	// Convert markdown to html
	var buf bytes.Buffer
	if err := goldmark.Convert(content, &buf); err != nil {
		fmt.Println(err)
	}

	var buf2 bytes.Buffer
	err = layoutTemplate.Execute(&buf2, Data{
		Title:       "Aleksandr Yakunichev",
		Description: "Aleksandr Yakunichev website",
		Content:     template.HTML(buf.Bytes()),
	})
	if err != nil {
		fmt.Println(err)
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/javascript", js.Minify)

	result, err := m.Bytes("text/html", buf2.Bytes())
	if err != nil {
		fmt.Println(err)
	}

	filename := filepath.Base(path)
	err = ioutil.WriteFile(filepath.Join(BuildDirectory, "e", strings.TrimSuffix(filename[11:len(filename)], ".md")+".html"), result, 0644)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("✓ File %s is written\n", strings.TrimSuffix(filename[11:len(filename)], ".md")+".html")

	wg.Done()
}

const (
	StaticDirectory    = "./static"
	BuildDirectory     = "./build"
	EventsDirectory    = "./events"
	TemplatesDirectory = "./templates"
)

type Event struct {
	Slug  string
	Title string
	Date  time.Time
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

	markdownFilenames := getMarkdownFilenames(EventsDirectory)
	fmt.Printf("! Found %d markdown files in events directory\n", len(markdownFilenames))

	// Layout
	layoutTemplate := template.Must(template.ParseFiles(filepath.Join(TemplatesDirectory, "./layout.tmpl.html")))

	wg := sync.WaitGroup{}
	for _, markdownFilename := range markdownFilenames {
		wg.Add(1)
		go transformMarkdownToHTML(markdownFilename, *layoutTemplate, &wg)
	}
	wg.Wait()

	var events []Event
	for _, markdownFilename := range markdownFilenames {
		filename := strings.TrimSuffix(filepath.Base(markdownFilename), ".md")
		t, err := time.Parse("2006-01-02", filename[0:10])
		if err != nil {
			panic(err)
		}
		events = append(events, Event{
			Slug:  filename[11:len(filename)],
			Title: filename[11:len(filename)],
			Date:  t,
		})
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.After(events[j].Date)
	})

	var buf bytes.Buffer
	indexTemplate := template.Must(template.ParseFiles(filepath.Join(TemplatesDirectory, "./index.tmpl.html")))
	err = indexTemplate.Execute(&buf, struct{ Events []Event }{Events: events})
	if err != nil {
		panic(err)
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/javascript", js.Minify)

	var buf2 bytes.Buffer
	err = layoutTemplate.Execute(&buf2, Data{
		Title:       "Aleksandr Yakunichev",
		Description: "Aleksandr Yakunichev website",
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
	fmt.Println("! Clean up static directory")
	err = os.RemoveAll(filepath.Join(BuildDirectory, StaticDirectory))
	if err != nil {
		fmt.Println(err)
	}
	err = os.MkdirAll(filepath.Join(BuildDirectory, StaticDirectory), os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}
	err = CopyDirectory(StaticDirectory, filepath.Join(BuildDirectory, StaticDirectory))
	if err != nil {
		panic(err)
	}
	fmt.Println("✓ Directody static copied to build/static")
}
