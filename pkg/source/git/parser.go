package git

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sivchari/keel/pkg/core"
)

// resourceDoc represents a raw resource document.
type resourceDoc struct {
	APIVersion  string            `yaml:"apiVersion" json:"apiVersion"`
	Kind        string            `yaml:"kind" json:"kind"`
	Metadata    metadataDoc       `yaml:"metadata" json:"metadata"`
	Spec        map[string]any    `yaml:"spec" json:"spec"`
	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
}

type metadataDoc struct {
	Name       string `yaml:"name" json:"name"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Generation int64  `yaml:"generation" json:"generation"`
}

func (s *Source) parseFile(relPath string) ([]*core.Resource, error) {
	w, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	root := w.Filesystem.Root()
	fullPath := filepath.Join(root, filepath.Clean(relPath))

	if !strings.HasPrefix(fullPath, root) {
		return nil, fmt.Errorf("path traversal detected: %s", relPath)
	}

	data, err := os.ReadFile(fullPath) //#nosec G304 -- path is validated above
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(relPath))

	switch ext {
	case ".yaml", ".yml":
		return s.parseYAML(data, relPath)
	case ".json":
		return s.parseJSON(data, relPath)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func (s *Source) parseYAML(data []byte, sourcePath string) ([]*core.Resource, error) {
	var resources []*core.Resource

	decoder := yaml.NewDecoder(bufio.NewReader(bytes.NewReader(data)))

	for {
		var doc resourceDoc
		if err := decoder.Decode(&doc); err != nil {
			if err.Error() == "EOF" {
				break
			}

			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		if doc.APIVersion == "" || doc.Kind == "" {
			continue
		}

		res := docToResource(&doc, sourcePath)
		resources = append(resources, res)
	}

	return resources, nil
}

func (s *Source) parseJSON(data []byte, sourcePath string) ([]*core.Resource, error) {
	data = bytes.TrimSpace(data)

	if len(data) == 0 {
		return nil, nil
	}

	if data[0] == '[' {
		var docs []resourceDoc
		if err := json.Unmarshal(data, &docs); err != nil {
			return nil, fmt.Errorf("failed to decode JSON array: %w", err)
		}

		var resources []*core.Resource

		for i := range docs {
			if docs[i].APIVersion == "" || docs[i].Kind == "" {
				continue
			}

			res := docToResource(&docs[i], sourcePath)
			resources = append(resources, res)
		}

		return resources, nil
	}

	var doc resourceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	if doc.APIVersion == "" || doc.Kind == "" {
		return nil, nil
	}

	return []*core.Resource{docToResource(&doc, sourcePath)}, nil
}

func docToResource(doc *resourceDoc, sourcePath string) *core.Resource {
	return &core.Resource{
		Key: core.ResourceKey{
			APIVersion: doc.APIVersion,
			Kind:       doc.Kind,
			Namespace:  doc.Metadata.Namespace,
			Name:       doc.Metadata.Name,
		},
		Spec:        doc.Spec,
		Labels:      doc.Labels,
		Annotations: doc.Annotations,
		Generation:  doc.Metadata.Generation,
		SourcePath:  sourcePath,
	}
}
