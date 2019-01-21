/*
Copyright 2019 VMware, Inc.
SPDX-License-Identifier: BSD-2-Clause
*/

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// OOP stub
type baseContainer struct {
	pathEnvVar      string
	unsafePaths     bool
	debugMode       bool
	fileSet         map[string]bool
	folderSet       map[string]bool
	neededFolderSet map[string]bool
}

func (container baseContainer) lookEnvForFile(file string) (string, bool) {

	// split the PATH (":" separator)
	for _, folder := range strings.Split(container.pathEnvVar, ":") {
		newFile := folder + "/" + file

		// check if newFile is existing
		if newFileInfo, err := os.Lstat(newFile); err == nil {
			// newFile exists, but it could be a file or folder, we are looking only for files
			// so if this is a folder, ignore and continue as we are expecting a file here
			if newFileInfo.Mode()&os.ModeDir != 0 {
				continue
			}
			return newFile, true
		}

	}

	return "", false
}

func (container baseContainer) addToSetsFromPath(pathString string) {

	// hardcoded paths to ignore
	ignorePaths := []string{"/dev", "/proc", "/sys", "/var/lib/docker"}
	// if the user has decided to enable unsafe archiving, disable all ignorePaths
	if container.unsafePaths {
		ignorePaths = []string{}
	}

	// normalize pathString that is in input to the function
	normalizedPathString := filepath.Clean(pathString)

	// check if normalizedPathString points to a symlink
	if isSym, err := IsSymlink(normalizedPathString); isSym && err == nil {

		// check if the symlink is in a path we want to ignore
		for _, prefix := range ignorePaths {
			if strings.HasPrefix(normalizedPathString, prefix+"/") {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring file: %s\n", normalizedPathString)
				}
				return
			}
		}

		// it's not in the ignorePaths set, so add the symlink to fileSet
		container.fileSet[normalizedPathString] = true
		// now read where this file is pointing to
		linkPath, _ := os.Readlink(normalizedPathString)
		if !path.IsAbs(linkPath) {
			// I got a relative link, create the full path
			index := strings.LastIndexByte(normalizedPathString, '/')
			val := normalizedPathString[:index] + "/" + linkPath
			linkPath = val
		}
		if container.debugMode {
			fmt.Printf("[addToSetsFromPath]: found symlink %s to %s\n", normalizedPathString, linkPath)
		}
		container.addToSetsFromPath(linkPath)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// now handle the case when normalizedPathString is not a symlink
	// check if normalizedPathString is a directory
	if val, err := IsDir(normalizedPathString); val && err == nil {
		for _, prefix := range ignorePaths {
			// ignore if normalizedPathString begins with "/prefix/" or it is exactly "/prefix"
			if strings.HasPrefix(normalizedPathString, prefix+"/") ||
				prefix == normalizedPathString {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring folder %s\n", normalizedPathString)
				}
				return
			}
		}
		// not in ignore list, add the folder
		container.folderSet[normalizedPathString] = true
	} else if !val && err == nil {
		// normalizedPathString is not a folder (it's a regular file)
		for _, prefix := range ignorePaths {
			// ignore if in ignore list
			if strings.HasPrefix(normalizedPathString, prefix+"/") {
				if container.debugMode {
					fmt.Printf("[addToSetsFromPath]: ignoring file %s\n", normalizedPathString)
				}
				return
			}
		}
		// not in the ignore list, add the file
		if container.debugMode {
			fmt.Printf("[addToSetsFromPath]: adding %s\n", normalizedPathString)
		}
		container.fileSet[normalizedPathString] = true
	} else {
		// we shouldn't end here
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func (container baseContainer) finalize() []string {

	// for each folder, check if a file is implicitly referencing it
	// i.e. /foo/bar.txt is implicitly referencing /foo folder
	// so /foo explicit creation is not needed
	for folder := range container.folderSet {
		if IsFolderNeeded(folder, container.fileSet, container.folderSet) {
			container.neededFolderSet[folder] = true
		} else {
			if container.debugMode {
				fmt.Printf("[finalize]: unneded folder %s\n", folder)
			}
		}
	}

	// some containers might decide to write to /tmp after startup (i.e. /tmp/app.lck)
	// microdump.lua will capture this action, but the file list might end up containing files
	// that do not exist in the base image, leading to a completely missing /tmp folder
	// check for /tmp folder for environment consistency
	found_tmp := false
	for k, _ := range container.fileSet {
		if strings.HasPrefix(k, "/tmp/") {
			found_tmp = true
			break
		}
	}

	// if not found, search for an implicitly defined /tmp in container.neededFolderSet
	if !found_tmp {
		for k, _ := range container.neededFolderSet {
			// be sure that we are not including a folder like "/tmpfoo"
			if k == "/tmp" || k == "/tmp/" {
				found_tmp = true
				break
			}
		}
		// check again now out of the for loop
		// if still not found, add it to the container.neededFolderSet
		if !found_tmp {
			container.neededFolderSet["/tmp/"] = true
			if container.debugMode {
				fmt.Println("[finalize]: adding explictly /tmp/ folder")
			}
		}
	}

	// now merge everything
	allPaths := make([]string, len(container.neededFolderSet)+len(container.fileSet))
	i := 0
	for key := range container.neededFolderSet {
		allPaths[i] = key
		i++
	}
	for key := range container.fileSet {
		allPaths[i] = key
		i++
	}
	return allPaths
}


// OOP constructor stub
func newBaseContainer(pathEnvVar string, unsafePaths bool, debugMode bool) baseContainer {
	return baseContainer{pathEnvVar, unsafePaths, debugMode, make(map[string]bool), make(map[string]bool), make(map[string]bool)}
}

// main
func main() {

	var pathEnvVar string
	var interp string
	var ok bool
	var err error

	// read arguments
	inputFlag := flag.String("i", "", "input file")
	outputFlag := flag.String("o", "rootfs.tar", "output file")
	interpFlag := flag.String("x", "/bin/sh", "ELF executable to read INTERP section from")
	unsafeFlag := flag.Bool("u", false, "unsafe archiving, disable hardcoded checks")
	debugFlag := flag.Bool("d", false, "debug mode (verbose output)")
	packageFlag := flag.String("p", "", "gather package information with detected pkg managers")
	flag.Parse()

	// check that string flags are set correctly, do not allow for empty "" strings
	// but do not check for packageFlag (an empty one means disable)
	if *inputFlag == "" {
		fmt.Fprintln(os.Stderr, "input file cannot be empty")
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}
	if *outputFlag == "" {
		fmt.Fprintln(os.Stderr, "output file cannot be empty")
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}
	if *interpFlag == "" {
		fmt.Fprintln(os.Stderr, "ELF executable cannot be empty")
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}

	// print flags values if in debug mode
	if *debugFlag {
		fmt.Printf("[main]: input file set to %s\n", *inputFlag)
		fmt.Printf("[main]: output file set to %s\n", *outputFlag)
		fmt.Printf("[main]: interp file set to %s\n", *interpFlag)
		fmt.Printf("[main]: unsafe archiving set to %t\n", *unsafeFlag)
		if *packageFlag == "" {
			fmt.Printf("[main]: package information file not set\n")
		} else {
			fmt.Printf("[main]: package information file set to %s\n", *packageFlag)
		}
		fmt.Printf("[main]: debug mode set to true\n")
	}

	// read first the PATH env variable
	pathEnvVar, ok = os.LookupEnv("PATH")
	if !ok {
		fmt.Fprintln(os.Stderr, "couldn't retrieve PATH environment variable")
		return
	}

	// read the interpreter from the INTERP ELF section
	interp, err = GetInterpFromExec(*interpFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if *debugFlag {
		fmt.Printf("[main]: from interp %s found\n", interp)
	}

	// create a container - OOP skeleton
	container := newBaseContainer(pathEnvVar, *unsafeFlag, *debugFlag)

	// add the file read from interp section
	// independently of the list of files specified in input, this is fixed
	container.addToSetsFromPath(interp)

	// read the input file
	inputFile, err := os.Open(*inputFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer inputFile.Close()

	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		// pathString contains the input line
		pathString := scanner.Text()
		// check if pathString is not pointing to an existing file or folder
		if _, err := os.Lstat(pathString); err != nil {
			if os.IsNotExist(err) && !path.IsAbs(pathString) {
				// if not an abs path, we might have something relative (i.e. a "java" string)
				// try to figure out if "java" is a command looking into the environment
				foundPath, ok := container.lookEnvForFile(pathString)
				if ok {
					container.addToSetsFromPath(foundPath)
				}
			}
			// other error in os.Lstat or this line is complete "garbage", discard
			continue
		} else {
			// err is nil, pathString points to something, either file or folder
			container.addToSetsFromPath(pathString)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// before finalizing the container, perform pkg info gathering
	// IMPORTANT! pkg info retrieval is done on files only, not on folders
	if *packageFlag != "" {
		pkgInfoFile, err := os.Create(*packageFlag)
		defer pkgInfoFile.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		// we need to detect what package managers are in this container
		// for now, we will support only dpkg and rpm
		pkgMngrFound := false

		// dpkg support
		pkgMngrPath, ok := container.lookEnvForFile("dpkg")
		if ok {
			pkgMngrFound = true
			if *debugFlag {
				fmt.Printf("[main]: dpkg package manager detected\n")
			}
			pkgInfoFile.WriteString("dpkg package manager results:\n")
			for filePath, _ := range container.fileSet {
				if *debugFlag {
					fmt.Printf("[main]: executing %s -S %s\n", pkgMngrPath, filePath)
				}
				// the command we want to execute is "dpkg -S filePath"
				output, err := ExecCmd(pkgMngrPath, "-S", filePath)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					continue
				}
				pkgInfoFile.WriteString(output)
			}
			// pretty newline in case of multiple package managers inside a container
			pkgInfoFile.WriteString("\n")
		}

		// the following rpm block is not in an "else" block
		// if a container has multiple package managers, we will try to manage both

		// rpm support
		pkgMngrPath, ok = container.lookEnvForFile("rpm")
		if ok {
			pkgMngrFound = true
			if *debugFlag {
				fmt.Printf("[main]: rpm package manager detected\n")
			}
			pkgInfoFile.WriteString("rpm package manager results:\n")
			for filePath, _ := range container.fileSet {
				if *debugFlag {
					fmt.Printf("[main]: executing %s -qf %s\n", pkgMngrPath, filePath)
				}
				// the command we want to execute is "rpm -qf filePath"
				output, err := ExecCmd(pkgMngrPath, "-qf", filePath)
                                if err != nil {
                                        fmt.Fprintln(os.Stderr, err)
                                        continue
                                }
				// for rpm, add filePath info in output
                                pkgInfoFile.WriteString(output + " " + filePath)
			}
			// pretty printing
			pkgInfoFile.WriteString("\n")
		}
		// TODO add more package manager support
		if !pkgMngrFound && *debugFlag {
			fmt.Printf("[main]: warning! couldn't detect any known package manager\n")
		}
	}

	// now finalize the container and return a slice with all paths
	// finalize will remove all duplicates and redundancies in
	// container.fileSet and container.folderSet
	allPaths := container.finalize()

	// continue ...
	if *debugFlag {
		for _, v := range allPaths {
			fmt.Printf("[main]: adding to tar %s\n", v)
		}
	}

	// create the tarfile specified in outputFlag
	if err := WriteTar(*outputFlag, allPaths); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if *debugFlag {
		fmt.Printf("[main]: packing complete!\n")
	}
}
