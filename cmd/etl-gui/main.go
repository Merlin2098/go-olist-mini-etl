// Command etl-gui is a desktop frontend for the ETL pipeline in
// internal/pipeline. It lets the user pick a source CSV from the
// filesystem, choose a load destination (JSON Lines or Postgres), and
// run the pipeline without touching the command line.
package main

import (
	"context"
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"go_proy1/internal/pipeline"
)

const jsonOutputPath = "data/output/customers_summary.jsonl"

func main() {
	a := app.NewWithID("dev.go-proy1.etl-gui")
	w := a.NewWindow("Olist ETL")
	w.Resize(fyne.NewSize(560, 420))

	sourcePath := widget.NewEntry()
	sourcePath.SetPlaceHolder("No file selected")
	sourcePath.Disable() // display-only; selection happens via the file dialog

	browseBtn := widget.NewButton("Browse…", func() {
		fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if rc == nil {
				return // user cancelled
			}
			defer rc.Close()
			sourcePath.SetText(rc.URI().Path())
		}, w)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".csv"}))
		fd.Show()
	})

	targetGroup := widget.NewRadioGroup([]string{"JSON Lines", "PostgreSQL"}, nil)
	targetGroup.SetSelected("JSON Lines")
	targetGroup.Horizontal = true

	output := widget.NewMultiLineEntry()
	output.Wrapping = fyne.TextWrapWord
	output.Disable()

	logLine := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...) + "\n"
		fyne.Do(func() { output.SetText(output.Text + line) })
	}

	var runBtn *widget.Button
	runBtn = widget.NewButton("Run pipeline", func() {
		if sourcePath.Text == "" {
			dialog.ShowInformation("No file selected", "Choose a CSV file first.", w)
			return
		}

		target := pipeline.TargetJSONLines
		if targetGroup.Selected == "PostgreSQL" {
			target = pipeline.TargetPostgres
		}

		runBtn.Disable()
		output.SetText("")
		logLine("Running pipeline (source=%s, target=%s)…", filepath.Base(sourcePath.Text), target)

		go func() {
			result, err := pipeline.Run(context.Background(), pipeline.Options{
				SourcePath:     sourcePath.Text,
				Target:         target,
				JSONOutputPath: jsonOutputPath,
			})

			for _, rowErr := range result.Errors {
				logLine("row error: %v", rowErr)
			}
			if err != nil {
				logLine("FAILED: %v", err)
			} else {
				logLine("loaded %d unique customers (target=%s)", result.Count, target)
			}
			fyne.Do(func() { runBtn.Enable() })
		}()
	})

	form := container.NewVBox(
		widget.NewLabelWithStyle("Source CSV", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, browseBtn, sourcePath),
		widget.NewLabelWithStyle("Load target", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		targetGroup,
		runBtn,
	)

	content := container.NewBorder(form, nil, nil, nil, container.NewScroll(output))
	w.SetContent(content)
	w.ShowAndRun()
}
