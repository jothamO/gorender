package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

//go:embed templates/*
var templateFiles embed.FS

// templateData is injected into every template file during scaffolding.
type templateData struct {
	Name           string
	FPS            int
	DurationFrames int
	Width          int
	Height         int
	Year           int
}

func buildNew() *cobra.Command {
	var (
		templateName   string
		fps            int
		durationFrames int
		width          int
		height         int
	)

	cmd := &cobra.Command{
		Use:   "new <project-name>",
		Short: "Scaffold a new composition project",
		Long: `Creates a ready-to-run composition project with the gorender contract pre-wired.

Templates:
  vanilla   Pure HTML + JS. Zero dependencies. Single file. (default)
  svelte    Svelte 4. Compiled output, no runtime. Best for complex animations.
  react     React 18. For teams already on React.`,
		Example: `  gorender new my-intro
  gorender new my-intro --template svelte
  gorender new product-demo --template react --fps 24 --frames 720 --width 1920 --height 1080`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return scaffold(name, templateName, templateData{
				Name:           name,
				FPS:            fps,
				DurationFrames: durationFrames,
				Width:          width,
				Height:         height,
				Year:           time.Now().Year(),
			})
		},
	}

	cmd.Flags().StringVarP(&templateName, "template", "t", "vanilla", "template to use: vanilla, svelte, react")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second")
	cmd.Flags().IntVar(&durationFrames, "frames", 150, "total frame count")
	cmd.Flags().IntVar(&width, "width", 1080, "composition width in pixels")
	cmd.Flags().IntVar(&height, "height", 1920, "composition height in pixels")

	return cmd
}

func scaffold(projectName, tmplName string, data templateData) error {
	validTemplates := map[string]bool{"vanilla": true, "svelte": true, "react": true}
	if !validTemplates[tmplName] {
		return fmt.Errorf("unknown template %q — choose: vanilla, svelte, react", tmplName)
	}

	// Target directory.
	outDir := filepath.Join(".", projectName)
	if _, err := os.Stat(outDir); err == nil {
		return fmt.Errorf("directory %q already exists", outDir)
	}

	srcDir := "templates/" + tmplName

	// Walk the embedded template directory and copy each file.
	err := fs.WalkDir(templateFiles, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		// Compute destination path.
		rel, _ := filepath.Rel(srcDir, path)
		dst := filepath.Join(outDir, rel)

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		raw, err := fs.ReadFile(templateFiles, path)
		if err != nil {
			return err
		}

		// Execute Go template substitutions.
		tmpl, err := template.New(path).Delims("{{.", "}}").Parse(string(raw))
		if err != nil {
			// Not a template — write as-is.
			return os.WriteFile(dst, raw, 0644)
		}

		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer f.Close()

		return tmpl.Execute(f, data)
	})

	if err != nil {
		return fmt.Errorf("scaffolding: %w", err)
	}

	// Write a gorender.json composition file into the project.
	compJSON := buildCompositionJSON(projectName, tmplName, data)
	if err := os.WriteFile(filepath.Join(outDir, "gorender.json"), []byte(compJSON), 0644); err != nil {
		return err
	}

	// Print next steps.
	printNextSteps(projectName, tmplName, data)
	return nil
}

func buildCompositionJSON(name, tmpl string, data templateData) string {
	url := "http://localhost:5173" // vite default for svelte/react
	if tmpl == "vanilla" {
		url = fmt.Sprintf("file://%s/%s/composition.html", mustAbsCwd(), name)
	}
	return fmt.Sprintf(`{
  "url": "%s",
  "durationFrames": %d,
  "fps": %d,
  "width": %d,
  "height": %d,
  "output": {
    "path": "./%s.mp4",
    "crf": 20,
    "preset": "medium"
  }
}
`, url, data.DurationFrames, data.FPS, data.Width, data.Height, name)
}

func printNextSteps(name, tmpl string, data templateData) {
	secs := float64(data.DurationFrames) / float64(data.FPS)
	fmt.Printf("\n  ✓ Created %s/ (%s template)\n\n", name, tmpl)
	fmt.Printf("  Composition:  %d×%d px · %d frames · %.1fs @ %dfps\n\n",
		data.Width, data.Height, data.DurationFrames, secs, data.FPS)

	switch tmpl {
	case "vanilla":
		fmt.Printf("  1. Edit   %s/composition.html\n", name)
		fmt.Printf("  2. Render gorender render %s/gorender.json\n\n", name)

	case "svelte":
		fmt.Printf("  1. cd %s\n", name)
		fmt.Printf("  2. npm install\n")
		fmt.Printf("  3. npm run dev          # preview at localhost:5173\n")
		fmt.Printf("  4. Edit Composition.svelte\n")
		fmt.Printf("  5. gorender render gorender.json\n\n")

	case "react":
		fmt.Printf("  1. cd %s\n", name)
		fmt.Printf("  2. npm install\n")
		fmt.Printf("  3. npm run dev          # preview at localhost:5173\n")
		fmt.Printf("  4. Edit Composition.jsx\n")
		fmt.Printf("  5. gorender render gorender.json\n\n")
	}

	fmt.Printf("  Docs: https://github.com/makemoments/gorender\n\n")
}

func mustAbsCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	return strings.ReplaceAll(abs, `\`, "/")
}
