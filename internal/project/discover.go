// Package project descobre arquivos .java dentro de um diretório
package project

import (
	"io/fs"
	"path/filepath"
)

func Discover(root string) ([]string, error) {
	var results []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && (d.Name() == ".git" ||
			d.Name() == "target" ||
			d.Name() == "build") {
			return filepath.SkipDir
		}

		if !d.IsDir() && filepath.Ext(path) == ".java" {
			results = append(results, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}
