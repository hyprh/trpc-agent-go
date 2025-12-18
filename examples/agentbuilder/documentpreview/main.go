// This example demonstrates how to preview the extracted content from documents before chunking.
// It reads files and shows the complete text content extracted by the reader.
package main

import (
	"context"
	"fmt"
	"path/filepath"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	_ "trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/markdown"
	_ "trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/source/file"
)

type noChunking struct{}

func (n *noChunking) Chunk(doc *document.Document) ([]*document.Document, error) {
	return []*document.Document{doc}, nil
}

func main() {
	ctx := context.Background()

	markdownFile := filepath.Join("..", "test-data", "paper.md")
	pdfFile := filepath.Join("..", "test-data", "transformer.pdf")

	fmt.Println("=== Markdown Document Preview (without chunking) ===\n")
	fmt.Println("File:", markdownFile)
	previewDocument(ctx, markdownFile)

	fmt.Println("\n=== PDF Document Preview (without chunking) ===\n")
	fmt.Println("File:", pdfFile)
	previewDocument(ctx, pdfFile)
}

func previewDocument(ctx context.Context, filePath string) {
	src := file.New(
		[]string{filePath},
		file.WithName("Document Source"),
		file.WithCustomChunkingStrategy(&noChunking{}),
	)

	docs, err := src.ReadDocuments(ctx)
	if err != nil {
		fmt.Printf("Error reading documents: %v\n", err)
		return
	}

	fmt.Printf("\nTotal documents: %d\n\n", len(docs))

	for i, doc := range docs {
		fmt.Printf("--- Document %d ---\n", i+1)
		fmt.Printf("Name: %s\n", doc.Name)
		fmt.Printf("Content Length: %d characters\n", len(doc.Content))

		fmt.Println("\nMetadata:")
		for key, value := range doc.Metadata {
			fmt.Printf("  %s: %v\n", key, value)
		}

		contentPreview := doc.Content
		fmt.Printf("\nContent Preview:\n%s\n\n", contentPreview)

		if len(docs) > 1 && i >= 2 {
			fmt.Printf("... showing first 3 documents out of %d total documents\n", len(docs))
			break
		}
	}
}
