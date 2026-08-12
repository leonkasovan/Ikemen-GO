//go:build mugen

package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gopxl/beep/v2"
)

//go:embed resources/defaultMugenMotif.ini
var defaultMotif []byte

//go:embed assets.zip
var assetsZip []byte

// init marks this build as the mugen variant: it enables mugen-specific
// behavior (mugenBuild, consulted by config.go) and installs the real
// mugenAssets implementation. Build tags are file-scoped, so tagged files
// flip package-level flags/func-vars that runtime code consults instead.
func init() {
	mugenBuild = true
	mugenAssets = extractMugenAssets
}

// extractMugenAssets extracts the embedded engine assets into an existing
// Mugen game folder on first run. Detection: the folder has data/mugen.cfg
// (it's a Mugen game) but no external/ directory (engine assets not present
// yet). It is a no-op otherwise.
func extractMugenAssets() error {
	if FileExist("external") != "" || FileExist("data/mugen.cfg") == "" {
		return nil
	}
	if err := extractEmbed(assetsZip); err != nil {
		return err
	}
	LogDebug("[main] Mugen game detected. Assets extraction completed successfully.")
	return nil
}

// extractEmbed extracts all files from the embedded ZIP content into the
// current directory.
func extractEmbed(content []byte) error {
	zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return err
	}
	for _, file := range zipReader.File {
		err := func(f *zip.File) error {
			fileReader, err := f.Open()
			if err != nil {
				return err
			}
			defer fileReader.Close()
			path := filepath.FromSlash(f.Name)
			if f.FileInfo().IsDir() {
				return os.MkdirAll(path, os.ModePerm)
			}
			if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
				return err
			}
			outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer outFile.Close()
			_, err = io.Copy(outFile, fileReader)
			return err
		}(file)
		if err != nil {
			return fmt.Errorf("failed to extract %s: %w", file.Name, err)
		}
	}
	return nil
}

// xmpDecode is a stub for mugen builds. Module music (.xm/.mod/.it/.s3m)
// requires libxmp, which mugen builds do not link.
func xmpDecode(f io.ReadSeekCloser) (beep.StreamSeekCloser, beep.Format, error) {
	return nil, beep.Format{}, Error("module music (xm/mod/it/s3m) is not supported in mugen builds")
}
