package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var (
	templateDir    = flag.String("template-dir", "templates", "Directory containing templates; defaults to 'templates' next to sourcedir")
	staticDir      = flag.String("static-dir", "", "Directory containing static files that are copied to the output directory")
	withExtensions = flag.Bool("with-extensions", true, "Include file extensions when generating HTML")
	cleanOutput    = flag.Bool("clean-output", true, "Clean output directory before generating files")
)

func main() {
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("usage: %s sourcedir outdir", os.Args[0])
	}
	sourceDir := flag.Arg(0)
	outDir := flag.Arg(1)

	// Parse templates
	tdir := *templateDir
	if tdir == "" {
		tdir = filepath.Join(filepath.Dir(sourceDir), "templates")
	}
	tdir, err := filepath.Abs(tdir)
	if err != nil {
		log.Fatalf("error getting absolute path for %s: %v", tdir, err)
	}
	if st, err := os.Stat(tdir); err != nil || !st.IsDir() {
		log.Fatalf("template directory %s does not exist or is not a directory", tdir)
	} else {
		log.Printf("using templates from %s", tdir)
	}

	tmpls, err := loadTemplates(tdir)
	if err != nil {
		log.Fatalf("error loading templates: %v", err)
	}

	// Clean output directory
	if *cleanOutput {
		if err := cleanDirectory(outDir); err != nil {
			log.Fatalf("error cleaning output directory: %v", err)
		}
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			meta.Meta,
			extension.Table,
		),
	)
	gen := &mdGenerator{
		md:    md,
		tmpls: tmpls,
		pol:   bluemonday.UGCPolicy(),
	}

	// Walk the source directory and generate the output. In the case where
	// copying or generating a file results in an error, we store the error
	// and return nil to keep walking; this ensures that we discover as
	// many errors as possible, instead of exiting on the first one.
	var renderErrs []error
	err = filepath.WalkDir(sourceDir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // nothing to do; keep recursing
		}

		// If the file is not a markdown file, just copy it to the output directory
		if filepath.Ext(path) != ".md" {
			log.Printf("copying %s", path)
			dst := filepath.Join(outDir, path[len(sourceDir):])
			if err := copyFile(path, dst); err != nil {
				renderErrs = append(renderErrs, fmt.Errorf("error copying %s to %s: %w", path, dst, err))
				return nil
			}
			return nil
		}

		// Convert the markdown file to HTML in the same directory
		// structure
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("error getting relative path for %s: %w", path, err)
		}

		// Change the '.md' extension to '.html'
		relPath = relPath[:len(relPath)-len(filepath.Ext(relPath))]
		if *withExtensions {
			relPath = relPath + ".html"
		}

		// Ensure the destination directory exists
		fullDest := filepath.Join(outDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullDest), 0755); err != nil {
			return fmt.Errorf("error creating directory for %s: %w", fullDest, err)
		}

		log.Printf("converting %s -> %s", path, filepath.Join(outDir, relPath))
		if err := gen.convertMarkdownFile(outDir, relPath, path); err != nil {
			renderErrs = append(renderErrs, fmt.Errorf("error converting %s to %s: %w", path, fullDest, err))
			return nil
		}
		return nil
	})
	if err != nil || len(renderErrs) > 0 {
		renderErrs = append([]error{err}, renderErrs...)
		log.Fatalf("error walking source directory: %v", errors.Join(renderErrs...))
	}

	// Copy all static files to the output directory
	if *staticDir != "" {
		var copyErrors []error
		err = filepath.Walk(*staticDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(*staticDir, path)
			if err != nil {
				return fmt.Errorf("error getting relative path for %s: %v", path, err)
			}
			dst := filepath.Join(outDir, relPath)

			log.Printf("copying %s -> %s", path, dst)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				copyErrors = append(copyErrors, fmt.Errorf("error creating directory for %s: %v", dst, err))
				return nil
			}
			if err := copyFile(path, dst); err != nil {
				copyErrors = append(copyErrors, fmt.Errorf("error copying %s to %s: %v", path, dst, err))
				return nil
			}
			return nil
		})
		if err != nil || len(copyErrors) > 0 {
			copyErrors = append([]error{err}, copyErrors...)
			log.Fatalf("error walking static directory: %v", errors.Join(copyErrors...))
		}
	}

	log.Printf("done")
}

func copyFile(src, dst string) error {
	// Copy the file contents, mode, and times
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()

	if _, err := io.Copy(df, f); err != nil {
		return err
	}

	if err := df.Close(); err != nil {
		return err
	}

	if err := os.Chmod(dst, fi.Mode()); err != nil {
		return err
	}

	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

var skipCleanFilenames = map[string]bool{
	".gitignore": true,
}

// cleanDirectory will remove the contents of the given directory, but without
// removing the directory itself or certain files in the root of the directory.
func cleanDirectory(dir string) error {
	rootDir, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer rootDir.Close()

	entries, err := rootDir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if skipCleanFilenames[entry] {
			continue
		}

		path := filepath.Join(dir, entry)
		log.Printf("cleaning: %s", path)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

type templates struct {
	// layouts contains parsed layout templates, keyed by their name
	// (without file extensions).
	layouts map[string]*template.Template

	// funcs is the set of additional functions that we make available to
	// templates.
	funcs template.FuncMap
}

func loadTemplates(root string) (*templates, error) {
	// Parse each template in the 'layouts' subdirectory of the given
	// directory.
	layoutDir, err := os.ReadDir(filepath.Join(root, "layouts"))
	if err != nil {
		return nil, err
	}

	// If there are any "partials"–i.e. template fragments that can be used
	// in a layout–load them.
	var partials map[string]string
	if pdir, err := os.Open(filepath.Join(root, "partials")); err == nil {
		defer pdir.Close()
		entries, err := pdir.Readdirnames(-1)
		if err != nil {
			return nil, err
		}

		partials = make(map[string]string, len(entries))
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(root, "partials", entry))
			if err != nil {
				return nil, err
			}

			// Remove any file extension from the partial name, and
			// ensure it has a "_" prefix.
			partialName, _, _ := strings.Cut(entry, ".")
			if !strings.HasPrefix(partialName, "_") {
				partialName = "_" + partialName
			}
			partials[partialName] = string(data)
		}
	}

	ret := &templates{
		layouts: make(map[string]*template.Template, len(layoutDir)),
		funcs:   template.FuncMap{},
	}

	for _, entry := range layoutDir {
		layoutName, _, _ := strings.Cut(entry.Name(), ".")

		// Read the file so that we can parse it with a specific name.
		data, err := os.ReadFile(filepath.Join(root, "layouts", entry.Name()))
		if err != nil {
			return nil, err
		}

		tmpl, err := template.New(layoutName).
			Funcs(ret.funcs).
			Parse(string(data))
		if err != nil {
			return nil, err
		}

		// Add any partials by name.
		for name, content := range partials {
			if _, err := tmpl.New(name).Parse(content); err != nil {
				return nil, err
			}
		}

		ret.layouts[layoutName] = tmpl
	}
	return ret, nil
}

type renderData struct {
	// Title is the title of the rendered page, as displayed in the page's
	// <title> element.
	Title string
	// Content is the main body content for the layout.
	Content any
	// Path is the relative path to the file being rendered, under the
	// output directory.
	Path string

	// TODO: maybe 'Data any'?
}

func (t *templates) render(layout string, w io.Writer, data renderData) error {
	tmpl, ok := t.layouts[layout]
	if !ok {
		return fmt.Errorf("layout %q not found", layout)
	}

	// Create a new Template instance that includes our "overlay", which
	// defines all the blocks that are required for the layout template to
	// be rendered.
	var overlay strings.Builder
	fmt.Fprintln(&overlay, `{{define "content"}}{{ .Content }}{{end}}`)

	// If we have a non-empty Title attribute, override that block as well.
	if data.Title != "" {
		fmt.Fprintln(&overlay, `{{define "title"}}{{ .Title }}{{end}}`)
	}

	overlayTmpl, err := template.Must(tmpl.Clone()).Parse(overlay.String())
	if err != nil {
		return err
	}

	// Render to a buffer and then to the output file to ensure that we
	// don't write a half-valid file.
	var outBuf bytes.Buffer
	if err := overlayTmpl.ExecuteTemplate(&outBuf, "base", data); err != nil {
		return err
	}

	if _, err := outBuf.WriteTo(w); err != nil {
		return err
	}
	return nil
}

type mdGenerator struct {
	md    goldmark.Markdown
	tmpls *templates
	pol   *bluemonday.Policy
}

func (g *mdGenerator) convertMarkdownFile(outDir, relPath, src string) error {
	// Read the markdown file
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	outPath := filepath.Join(outDir, relPath)
	df, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer df.Close()

	// Parse the markdown file
	var buf bytes.Buffer
	context := parser.NewContext()
	if err := g.md.Convert(b, &buf, parser.WithContext(context)); err != nil {
		return err
	}
	metaData := meta.Get(context)

	// Sanitize the generated HTML.
	sanitized := template.HTML(g.pol.Sanitize(buf.String()))

	// Get the layout from the frontmatter.
	layout := "base"
	if t, ok := metaData["layout"].(string); ok {
		layout = t
	}

	// Load the title (if given)
	var title string
	if t, ok := metaData["title"].(string); ok {
		title = t
	}

	// Render the markdown file using the template
	if err := g.tmpls.render(layout, df, renderData{
		Title:   title,
		Content: sanitized,
		Path:    relPath,
	}); err != nil {
		return err
	}

	return nil
}
