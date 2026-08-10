package main

import (
	"context"
	"flag"
	"log"

	"go_proy1/internal/pipeline"
	"go_proy1/internal/timer"
)

const jsonOutputPath = "data/output/customers_summary.jsonl"

func main() {
	target := flag.String("target", "jsonl", "load destination: jsonl or postgres")
	source := flag.String("source", "data/olist_customers_dataset.csv", "path to the source CSV file")
	flag.Parse()

	defer timer.Track("pipeline")()

	result, err := pipeline.Run(context.Background(), pipeline.Options{
		SourcePath:     *source,
		Target:         pipeline.Target(*target),
		JSONOutputPath: jsonOutputPath,
	})
	for _, rowErr := range result.Errors {
		log.Printf("pipeline error: %v", rowErr)
	}
	if err != nil {
		log.Fatalf("load: %v", err)
	}

	log.Printf("loaded %d unique customers (target=%s)", result.Count, *target)
}
