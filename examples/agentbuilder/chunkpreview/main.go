// This example demonstrates how to use the file source to preview Markdown document chunks.
// It reads a Markdown file and splits it into chunks with custom settings (500 char chunks, 50 char overlap).
//
// The example uses paper.md from the test-data directory, which contains content about AI agents.
// Each chunk includes metadata such as chunk index, chunk size, markdown header path, and content preview.
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	_ "trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/source/file"
)

func main() {
	ctx := context.Background()

	markdownFile := filepath.Join("..", "test-data", "paper.md")
	pdfFile := filepath.Join("..", "test-data", "transformer.pdf")

	fmt.Println("=== Markdown Chunk Preview Example ===\n")
	fmt.Println("File:", markdownFile)
	previewChunk(ctx, markdownFile)

	fmt.Println("=== Pdf Chunk Preview Example ===\n")
	fmt.Println("File:", pdfFile)
	previewChunk(ctx, pdfFile)
}

func previewChunk(ctx context.Context, filePath string) {
	src := file.New(
		[]string{filePath},
		file.WithName("Markdown Source"),
		file.WithChunkSize(500),
		file.WithChunkOverlap(50),
	)

	docs, err := src.ReadDocuments(ctx)
	if err != nil {
		fmt.Printf("Error reading documents: %v\n", err)
		return
	}

	fmt.Printf("\nTotal chunks: %d\n\n", len(docs))

	keepKeys := map[string]bool{
		"trpc_agent_go_markdown_header_path": true,
		"trpc_agent_go_uri":                  true,
		"trpc_agent_go_source_name":          true,
		"trpc_agent_go_chunk_index":          true,
		"trpc_agent_go_chunk_type":           true,
		"trpc_agent_go_chunk_size":           true,
	}

	for i, doc := range docs {
		fmt.Printf("--- Chunk %d ---\n", i+1)
		fmt.Printf("Content Length: %d\n", len(doc.Content))

		fmt.Println("\nMetadata:")
		for key, value := range doc.Metadata {
			if strings.HasPrefix(key, "trpc_agent_go_") {
				if keepKeys[key] {
					fmt.Printf("  %s: %v\n", key, value)
				}
			} else {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}

		contentPreview := doc.Content
		// if len(contentPreview) > 200 {
		// 	contentPreview = contentPreview[:200] + "..."
		// }
		fmt.Printf("\nContent Preview:\n%s\n\n", contentPreview)

		if i >= 4 {
			fmt.Printf("... showing first 5 chunks out of %d total chunks\n", len(docs))
			break
		}
	}
}
