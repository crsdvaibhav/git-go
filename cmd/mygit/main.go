package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...

/*
os.Args: Gives the list of arguments passed in command line
fmt.Fprintf: Takes in a writer and a format string
os.Exit(): Exit program with exit status
range : Iterates over the data structure, returns index and value
os.MkdirAll takes in arguments one by one and makes directories, if error then return error
0755 : Means default permissions, that is readable/ excutable by everyone but writable by user only
[]byte : Is just string with extra capacity field
0644 : Readable by all by writable by user
os.WriteFile : Writes to the file
- refs: The master copy of all refs that live in your repository, be they for stashes, tags, remote tracking branches, or local branches.
- objects: Git’s internal warehouse of blobs, all indexed by SHAs
- HEAD: The current ref that you’re looking at. In most cases it’s probably refs/heads/master
*/

func hash_file(filePath string) ([20]byte, error) {
	fileContents, err := os.ReadFile(filePath)
	if err != nil {
		return [20]byte{}, err
	}

	//header
	header := fmt.Sprintf("blob %d\x00", len(fileContents))
	storeContents := append([]byte(header), fileContents...)

	//Get the SHA-1
	rawSha := sha1.Sum(storeContents)
	blobSha := fmt.Sprintf("%x", rawSha)
	blobPath := path.Join(".git", "objects", blobSha[:2], blobSha[2:])

	//zlib
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(storeContents)
	w.Close()

	// if file does not exist then create it, otherwise replace it
	if _, err := os.Stat(blobPath); os.IsNotExist(err) {
		if err := os.MkdirAll(path.Join(".git", "objects", blobSha[:2]), 0755); err != nil {
			return [20]byte{}, err
		}
	}
	if err := os.WriteFile(blobPath, b.Bytes(), 0644); err != nil {
		return [20]byte{}, err
	}

	return rawSha, nil
}

func hash_dir(rootPath string) ([20]byte, error) {
	files, err := os.ReadDir(rootPath)
	if err != nil {
		return [20]byte{}, err
	}
	var entries []string
	for _, file := range files {
		// skip .git directory
		if file.Name() == ".git" {
			continue
		}
		var sha [20]byte
		mode := 0o100644
		fullFilePath := path.Join(rootPath, file.Name())
		if file.IsDir() {
			treeSha, err := hash_dir(fullFilePath)
			if err != nil {
				return [20]byte{}, err
			}
			sha = treeSha
			// octal representation of directory (octal type)
			mode = 0o040000
		} else {
			// get file sha
			fileSha, err := hash_file(fullFilePath)
			if err != nil {
				return [20]byte{}, err
			}
			sha = fileSha
			// octal representation of file (regular type)
			mode = 0o100644
		}
		entries = append(entries, fmt.Sprintf("%o %s\x00%s", mode, file.Name(), sha)) //Add NULL byte at the end of each
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i][strings.IndexByte(entries[i], ' ')+1:] < entries[j][strings.IndexByte(entries[j], ' ')+1:]
	}) //Sort alphabetically

	// create tree object
	var b bytes.Buffer
	var contents bytes.Buffer
	for _, entry := range entries {
		contents.WriteString(entry)
	}
	header := fmt.Sprintf("tree %d\x00", contents.Len())
	b.WriteString(header)     //Write header
	b.Write(contents.Bytes()) //Then contents
	var rawSha = sha1.Sum(b.Bytes())
	treeSha := fmt.Sprintf("%x", rawSha)
	treePath := path.Join(".git", "objects", treeSha[:2], treeSha[2:])

	// create zlib writer
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(b.Bytes())
	w.Close()

	// if file does not exist then create it, otherwise replace it
	if _, err := os.Stat(treePath); os.IsNotExist(err) {
		if err := os.MkdirAll(path.Join(".git", "objects", treeSha[:2]), 0755); err != nil {
			return [20]byte{}, err
		}
	}
	if err := os.WriteFile(treePath, compressed.Bytes(), 0644); err != nil {
		return [20]byte{}, err
	}
	return rawSha, nil
}

func commit_tree(sha_tree string, sha_parent string, message string) ([20]byte, error) {
	var commit bytes.Buffer
	commit.WriteString(fmt.Sprintf("tree %s\n", sha_tree)) //Add tree SHA

	if sha_parent != "" {
		commit.WriteString(fmt.Sprintf("parent %s\n", sha_parent)) //Add parent SHA
	}

	timestamp := time.Now().Unix()
	timezone_offset := time.Now().Format("-0700")
	author := fmt.Sprintf("author Bocchi! The Rock <bocchi@therock.com> %d %s", timestamp, timezone_offset)
	committer := fmt.Sprintf("committer Bocchi! The Rock <bocchi@therock.com> %d %s", timestamp, timezone_offset)
	commit.WriteString(fmt.Sprintf("author %s\n", author))       //Add author
	commit.WriteString(fmt.Sprintf("committer %s\n", committer)) //Add committer

	if message != "" {
		commit.WriteString(fmt.Sprintf("\n%s\n", message))
	}

	var raw_sha = sha1.Sum(commit.Bytes())
	commit_sha := fmt.Sprintf("%x", raw_sha)
	commitPath := path.Join(".git", "objects", commit_sha[:2], commit_sha[2:])

	//header
	header := fmt.Sprintf("commit %d\x00", commit.Len())
	var b bytes.Buffer
	b.WriteString(header)
	b.Write(commit.Bytes())

	//zlib
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(b.Bytes())
	w.Close()

	// if file does not exist then create it, otherwise replace it
	if _, err := os.Stat(commitPath); os.IsNotExist(err) {
		if err := os.MkdirAll(path.Join(".git", "objects", commit_sha[:2]), 0755); err != nil {
			return [20]byte{}, err
		}
	}
	if err := os.WriteFile(commitPath, compressed.Bytes(), 0644); err != nil {
		return [20]byte{}, err
	}

	return raw_sha, nil
}

func main() {
	if len(os.Args) < 2 { //If len of anrguments is not valid
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	//Switch case statement
	switch command := os.Args[1]; command { //On the first argument passed
	case "init": //If init
		//Make directory structure
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/master\n") //Contents of file
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory") //Send response

	case "cat-file":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit cat-file -p <blob_hash>\n")
			os.Exit(1)
		}

		blob_sha := os.Args[3]                                               //Get the SHA
		blobPath := path.Join(".git", "objects", blob_sha[:2], blob_sha[2:]) //Get the path

		reader, err := os.Open(blobPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "File not found: %s\n", err)
			os.Exit(1)
		}

		zlibreader, err := zlib.NewReader(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file: %s\n", err)
			os.Exit(1)
		}

		contents, err := ioutil.ReadAll(zlibreader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		}

		//Remove the "blob size\0"
		index := bytes.IndexByte(contents, 0) + 1
		data := string(contents[index:])
		fmt.Printf("%s\n", data)

	case "hash-object":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit hash-object -w <file>\n")
			os.Exit(1)
		}

		filename := os.Args[3]
		dat, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error unable to read file: %s\n", err)
		}
		data := string(dat)                             //contents
		header := fmt.Sprintf("blob %d\x00", len(data)) //Header
		content := append([]byte(header), data...)      //Added header to content

		sha_data := fmt.Sprintf("%x", sha1.Sum(content)) //sha1

		var compresed_data bytes.Buffer
		w := zlib.NewWriter(&compresed_data)
		w.Write([]byte(content))
		w.Close()

		filepath := path.Join(".git", "objects", sha_data[:2], sha_data[2:])

		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			if err := os.MkdirAll(path.Join(".git", "objects", sha_data[:2]), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating folder: %s\n", err)
				os.Exit(1)
			}
		}
		if err := os.WriteFile(filepath, compresed_data.Bytes(), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("%s\n", sha_data)

	case "ls-tree":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit ls-tree --name-only <tree-SHA>\n")
			os.Exit(1)
		}

		tree_sha := os.Args[3]
		treePath := path.Join(".git", "objects", tree_sha[:2], tree_sha[2:])

		reader, err := os.Open(treePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding tree : %s\n", err)
			os.Exit(1)
		}

		zlibreader, err := zlib.NewReader(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing tree : %s\n", err)
			os.Exit(1)
		}

		contents, err := ioutil.ReadAll(zlibreader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
			os.Exit(1)
		}

		header_index := bytes.IndexByte(contents, 0)
		contents = contents[header_index+1:]

		var paths []string

		for len(contents) > 0 {
			space_index := strings.IndexByte(string(contents), ' ')
			mode := contents[:space_index]
			contents = contents[len(mode)+1:]

			null_index := strings.IndexByte(string(contents), 0)
			path := contents[:null_index]
			contents = contents[len(path)+1:]

			path_sha := contents[:20]
			contents = contents[len(path_sha):]

			paths = append(paths, string(path))
		}

		for _, name := range paths {
			fmt.Println(name)
		}

	case "write-tree":
		if len(os.Args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: mygit write-tree\n")
			os.Exit(1)
		}
		// find directory where .git is located
		gitDir, err := os.Getwd() //Returns path to current directory
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %s\n", err)
			os.Exit(1)
		}
		for {
			if _, err := os.Stat(path.Join(gitDir, ".git")); err == nil { //Id this dir has .git
				break
			}
			gitDir = path.Dir(gitDir) //Goes one dir up
		}
		treeSha, err := hash_dir(gitDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error hashing tree: %s\n", err)
			os.Exit(1)
		}
		// print sha
		fmt.Printf("%x\n", treeSha)

	case "commit-tree":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: mygit commit-tree <tree_sha> -p <commit_sha> -m <message>\n")
			os.Exit(1)
		}

		tree_sha := os.Args[2]
		parent_sha := ""
		message := ""

		if len(os.Args) == 5 {
			if os.Args[3] == "-p" {
				parent_sha = os.Args[4]
			} else if os.Args[3] == "-m" {
				message = os.Args[4]
			} else {
				fmt.Fprintf(os.Stderr, "usage: mygit commit-tree <tree_sha> -p <commit_sha> -m <message>\n")
				os.Exit(1)
			}
		} else if len(os.Args) == 7 {
			if os.Args[3] == "-p" {
				parent_sha = os.Args[4]
				message = os.Args[6]
			} else {
				fmt.Fprintf(os.Stderr, "usage: mygit commit-tree <tree_sha> -p <commit_sha> -m <message>\n")
				os.Exit(1)
			}
		}
		commit_sha, err := commit_tree(tree_sha, parent_sha, message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error committing tree: %s\n", err)
			os.Exit(1)
		}
		// print sha
		fmt.Printf("%x\n", commit_sha)

	default: //If anything else
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}

//Parent SHA 90f1b459a5271c631b525dfb71364b715d9a9f9a
//Tree SHA b5ea78b78579c2ffad30bd9e1b78000dbfcbcc74
