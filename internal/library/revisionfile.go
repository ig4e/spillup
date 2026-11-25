package library

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
)

var errRevisionType = errors.New("document history requires regular files and directories")
var errRevisionText = errors.New("document history supports UTF-8 text up to 128 KiB")

func childRevisionRoot(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errRevisionType
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	checked, err := child.Stat(".")
	if err != nil || !os.SameFile(info, checked) {
		child.Close()
		return nil, errRevisionType
	}
	return child, nil
}

func revisionParent(root *os.Root, path string, create bool) (*os.Root, string, error) {
	if !documentPath(path) {
		return nil, "", errors.New("unsupported library document")
	}
	if path == Instructions {
		return root, path, nil
	}
	current := root
	for _, part := range strings.Split(path, "/")[:2] {
		if create {
			if err := current.Mkdir(part, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
				if current != root {
					current.Close()
				}
				return nil, "", err
			}
		}
		next, err := childRevisionRoot(current, part)
		if current != root {
			current.Close()
		}
		if err != nil {
			return nil, "", err
		}
		current = next
	}
	return current, skillFile, nil
}
func currentDocument(root *os.Root, path string) ([]byte, error) {
	parent, base, err := revisionParent(root, path, false)
	if err != nil {
		return nil, err
	}
	if parent != root {
		defer parent.Close()
	}
	data, _, err := readRevisionFile(parent, base, revisionBytes)
	if err != nil {
		return nil, err
	}
	if !documentText(data) {
		return nil, errRevisionText
	}
	return data, nil
}

func readRevisionFile(root *os.Root, name string, limit int64) ([]byte, fs.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errRevisionType
	}
	if info.Size() > limit {
		return nil, nil, errRevisionText
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	checked, err := file.Stat()
	if err != nil || !os.SameFile(info, checked) {
		return nil, nil, errRevisionType
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > limit {
		return nil, nil, errRevisionText
	}
	return data, info, nil
}

func writeRevisionFile(root *os.Root, name string, data []byte, mode fs.FileMode) error {
	tmp := ".revision-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(tmp, name)
}
