module example

go 1.24.4

replace (
	trpc.group/trpc-go/trpc-agent-go => ../../
	trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf => ../../knowledge/document/reader/pdf
	trpc.group/trpc-go/trpc-agent-go/knowledge/embedder/gemini => ../../knowledge/embedder/gemini
	trpc.group/trpc-go/trpc-agent-go/knowledge/ocr/tesseract => ../../knowledge/ocr/tesseract
	trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore/elasticsearch => ../../knowledge/vectorstore/elasticsearch
	trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore/milvus => ../../knowledge/vectorstore/milvus
	trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore/pgvector => ../../knowledge/vectorstore/pgvector
	trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore/tcvector => ../../knowledge/vectorstore/tcvector
	trpc.group/trpc-go/trpc-agent-go/storage/milvus => ../../storage/milvus
	trpc.group/trpc-go/trpc-agent-go/storage/postgres => ../../storage/postgres
)

require (
	trpc.group/trpc-go/trpc-agent-go v0.2.0
	trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/pdf v0.0.0-00010101000000-000000000000
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.0 // indirect
	github.com/hhrutter/tiff v1.0.2 // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/pdfcpu/pdfcpu v0.11.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/yuin/goldmark v1.4.13 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/image v0.32.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	trpc.group/trpc-go/trpc-a2a-go v0.2.5-0.20251023030722-7f02b57fd14a // indirect
)
