package files

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type File struct {
	Path       string        `yaml:"path" validate:"required,abs_path_or_url"`
	Interval   time.Duration `yaml:"interval"`
	lastUpdate time.Time
	remote     bool
}

type Config map[string]*File

type FilesDiscoveryEngine struct {
	Config

	watcher     *fsnotify.Watcher
	filesByPath map[string]string
	buffer      map[string]string
}

func (f *FilesDiscoveryEngine) Setup() (err error) {
	f.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	f.filesByPath = make(map[string]string)
	f.buffer = make(map[string]string)
	for name, file := range f.Config {
		file.remote = !filepath.IsAbs(file.Path)
		f.updateDataFromFile(name)
		if file.Interval != 0 {
			continue
		}
		f.filesByPath[file.Path] = name
		err = f.watcher.Add(file.Path)
		if err != nil {
			return err
		}
	}

	return
}

func (f *FilesDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	defer f.watcher.Close()
	callback(maps.Clone(f.buffer), nil)

	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			hasUpdate := false
			for name, file := range f.Config {
				if file.Interval != 0 && time.Until(file.lastUpdate.Add(file.Interval)) < time.Second/2 {
					previous := f.buffer[name]
					err := f.updateDataFromFile(name)
					if err != nil {
						callback(nil, err)
						continue
					}
					hasUpdate = hasUpdate || f.buffer[name] != previous
				}
			}
			if hasUpdate {
				callback(maps.Clone(f.buffer), nil)
			}
		case event, ok := <-f.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				err := f.updateDataFromFile(f.filesByPath[event.Name])
				callback(maps.Clone(f.buffer), err)
			}
		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			callback(nil, err)
		}
	}
}

func (f *FilesDiscoveryEngine) updateDataFromFile(name string) error {
	file := f.Config[name]

	contents, err := file.Read()
	if err != nil {
		return err
	}

	f.buffer[name] = strings.TrimSpace(string(contents))
	file.lastUpdate = time.Now()

	return nil
}

func (f *File) Read() (contents []byte, err error) {
	if f.remote {
		response, err := http.Get(f.Path)
		if err != nil {
			return nil, fmt.Errorf("could not GET url %s: %w", f.Path, err)
		}
		contents, err = io.ReadAll(response.Body)
		if err != nil {
			return nil, fmt.Errorf("could not read body from response to GET %s: %w", f.Path, err)
		}
	} else {
		contents, err = os.ReadFile(f.Path)
		if err != nil {
			return nil, fmt.Errorf("could not read file %s: %w", f.Path, err)
		}
	}

	return contents, nil
}
