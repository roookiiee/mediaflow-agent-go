package vector

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Document struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

func LoadMarkdownDocuments(dir string) ([]Document, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	docs := make([]Document, 0, len(entries)*4)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, chunk := range splitParagraphs(string(body)) {
			docs = append(docs, Document{
				ID:     stableID(entry.Name() + "\n" + chunk),
				Source: entry.Name(),
				Text:   chunk,
			})
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Source == docs[j].Source {
			return docs[i].ID < docs[j].ID
		}
		return docs[i].Source < docs[j].Source
	})
	return docs, nil
}

func splitParagraphs(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	chunks := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len([]rune(item)) > 900 {
			item = string([]rune(item)[:900])
		}
		chunks = append(chunks, item)
	}
	return chunks
}

func stableID(text string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(text))
	value := int64(hash.Sum64() & 0x7fffffffffffffff)
	if value == 0 {
		return 1
	}
	return value
}
