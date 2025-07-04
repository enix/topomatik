package files

import (
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type File struct {
	Path string `yaml:"path"`
}

type Config map[string]File

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
		f.filesByPath[file.Path] = name
		err = f.watcher.Add(file.Path)
		f.updateDataFromFile(name)
		if err != nil {
			return err
		}
	}

	return
}

func (f *FilesDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	go func() {
		defer f.watcher.Close()
		callback(f.getDataFromBuffer(), nil)

		for {
			select {
			case event, ok := <-f.watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					err := f.updateDataFromFile(f.filesByPath[event.Name])
					callback(f.getDataFromBuffer(), err)
				}
			case err, ok := <-f.watcher.Errors:
				if !ok {
					return
				}
				callback(nil, err)
			}
		}
	}()
}

func (f *FilesDiscoveryEngine) updateDataFromFile(name string) error {
	file := f.Config[name]
	contents, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("could not read file %s: %w", file.Path, err)
	}

	f.buffer[name] = strings.TrimSpace(string(contents))

	return nil
}

// getDataFromBuffer copies the contents of the buffer into a new map and returns it.
// The goal is to prevent modification of the original buffer by the caller.
func (f *FilesDiscoveryEngine) getDataFromBuffer() map[string]string {
	data := make(map[string]string)
	for name, value := range f.buffer {
		data[name] = value
	}
	return data
}
