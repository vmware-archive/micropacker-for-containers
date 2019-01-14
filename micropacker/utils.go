/*
Copyright 2019 VMware, Inc.
SPDX-License-Identifier: BSD-2-Clause
*/

package main

import (
	"archive/tar"
	"bytes"
	"debug/elf"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetInterpFromExec(file string) (string, error) {
	var err error
	var fileInfo os.FileInfo
	if fileInfo, err = os.Stat(file); err == nil {
		if fileInfo.Mode()&os.ModeType == 0 {
			elfFile, _ := elf.Open(file)
			defer elfFile.Close()
			// TODO check that file is ELF?
			section := elfFile.Section(".interp")
			if section != nil {
				var interp []byte
				if interp, err = section.Data(); err == nil {
					return string(interp[:len(interp)-1]), nil
				}
				return "", err
			}
			return "", errors.New("[GetInterpFromExec]: couldn't find the interp section")
		}
		return "", errors.New("[GetInterpFromExec]: " + file + " is not a regular file")
	}
	return "", err
}

func IsDir(filename string) (bool, error) {
	var err error
	var fileInfo os.FileInfo
	if fileInfo, err = os.Lstat(filename); err == nil {
		if fileInfo.Mode()&os.ModeDir != 0 {
			return true, nil
		}
		return false, nil
	}
	return false, err
}

func IsSymlink(filename string) (bool, error) {
	var err error
	var fileInfo os.FileInfo
	if fileInfo, err = os.Lstat(filename); err == nil {
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
		return false, nil
	}
	return false, err
}

func IsFolderNeeded(folder string, fileSet map[string]bool, folderSet map[string]bool) bool {
	for fileEntry := range fileSet {
		if strings.HasPrefix(fileEntry, folder) {
			return false
		}
	}
	for folderEntry := range folderSet {
		if strings.HasPrefix(folderEntry, folder) && folder != folderEntry {
			return false
		}
	}
	return true
}

func WriteTar(tarPath string, paths []string) error {
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()
	tarWriter := tar.NewWriter(tarFile)
	defer tarWriter.Close()
	for _, path := range paths {
		if err := addToTar(tarWriter, path); err != nil {
			return err
		}
	}
	return nil
}

func addToTar(tarWriter *tar.Writer, path string) error {
	return filepath.Walk(path, func(fullPath string, fileInfo os.FileInfo, err error) error {
		// filepath.Walk failed
		if err != nil {
			return err
		}
		// we need to add the proper link in tar.FileInfoHeader
		var link string
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(fullPath)
			if err != nil {
				return err
			}
		}
		// link is empty if not symlink
		tarHeader, err := tar.FileInfoHeader(fileInfo, link)
		if err != nil {
			return err
		}
		// put the fullpath back in tarHeader.Name
		tarHeader.Name = fullPath
		err = tarWriter.WriteHeader(tarHeader)
		if err != nil {
			return err
		}
		if tarHeader.Typeflag == tar.TypeReg {
			file, err := os.Open(fullPath)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.CopyN(tarWriter, file, fileInfo.Size())
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func ExecCmd(executable string, executableArgs ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.Command(executable, executableArgs...)
	cmd.Stdout = &out
        err := cmd.Run()
	return out.String(), err
}
