package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	flags "github.com/jessevdk/go-flags"
)

type Options struct {
	ClusterId        string `short:"c" long:"cluster-id" description:"Cluster ID"`
	LoginScript      string `short:"l" long:"login-script" description:"OCM login script to execute in a loop in ocb every 30 seconds"`
	ExportKubeConfig bool   `short:"k" long:"export-kubeconfig" description:"Output export kubeconfig statement, to use environment outside of directory"`
	ResetEnv         bool   `short:"r" long:"reset" description:"Reset environment"`
	TempEnv          bool   `short:"t" long:"temp" description:"Delete environment on exit"`
	DeleteEnv        bool   `short:"d" long:"delete" description:"Delete environment"`
	Positional       struct {
		Alias string
	} `positional-args:"yes"`
}

type OcEnv struct {
	Path    string
	Exists  bool
	Options Options
}

func init() {
}

func main() {
	options := Options{}
	complete()
	flags.Parse(&options)

	if flag.CommandLine.NArg() > 0 {
		options.Positional.Alias = flag.Arg(0)
	}

	if options.ClusterId == "" && options.Positional.Alias == "" {
		flag.Usage()
		log.Fatal("ClusterId or Alias required")
	}

	if options.Positional.Alias == "" {
		log.Println("No Alias set, using cluster ID")
		options.Positional.Alias = options.ClusterId
	}

	env := OcEnv{
		Path:    os.Getenv("HOME") + "/ocenv/" + options.Positional.Alias,
		Options: options,
	}
	env.Setup()
	if options.DeleteEnv {
		env.Delete()
		return
	}
	if options.ExportKubeConfig {
		env.PrintKubeConfigExport()
		return
	}
	env.Migration()
	env.Start()
	if options.TempEnv {
		env.Delete()
	}

}

func (e *OcEnv) Setup() {
	if e.Options.ResetEnv {
		e.Delete()
	}
	e.ensureEnvDir()
	if !e.Exists || e.Options.ResetEnv {
		fmt.Println("Setting up environment...")
		e.createBins()
		e.ensureEnvVariables()
	}
}

func (e *OcEnv) PrintKubeConfigExport() {
	fmt.Printf("export KUBECONFIG=%s\n", e.Path+"/kubeconfig.json")
}

func (e *OcEnv) Migration() {
	if _, err := os.Stat(e.Path + "/.envrc"); err == nil {
		fmt.Println("Migrating from .envrc to .ocenv...")

		file, err := os.Open(e.Path + "/.envrc")
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "export CLUSTERID=") {
				e.Options.ClusterId = strings.ReplaceAll(line, "export CLUSTERID=", "")
				e.Options.ClusterId = strings.ReplaceAll(line, "\"", "")
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}

		e.ensureEnvVariables()

		os.Remove(e.Path + "/.envrc")
	}
}
func (e *OcEnv) Start() {
	shell := os.Getenv("SHELL")

	fmt.Print("Switching to OpenShift environment " + e.Options.Positional.Alias + "\n")
	fmt.Printf("%s %s\n", shell, e.Path+"/.ocenv")
	cmd := exec.Command(shell, "--rcfile", e.Path+"/.ocenv")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = e.Path
	_ = cmd.Run() // add error checking

	e.killChilds()

	fmt.Printf("Exited OpenShift environment\n")

}
func (e *OcEnv) killChilds() {
	file, err := os.Open(e.Path + "/.killpids")

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Nothing to kill")
			return
		}
		log.Fatalf("Failed to read file .killpids: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	scanner.Split(bufio.ScanLines)
	var text []string

	for scanner.Scan() {
		text = append(text, scanner.Text())
	}

	for _, pid := range text {
		fmt.Printf("Stopping process %s\n", pid)
		pidNum, err := strconv.Atoi(pid)
		if err != nil {
			log.Printf("failed to read PID %s, you may need to clean up manually: %v\n", pid, err)
		}
		err = syscall.Kill(pidNum, syscall.SIGTERM)
		if err != nil {
			log.Printf("failed to stop child processes %s, you may need to clean up manually: %v\n", pid, err)
		}
	}

	err = os.Remove(e.Path + "/.killpids")
	if err != nil {
		log.Printf("failed to delete .killpids, you may need to clean it up manually: %v\n", err)
	}

}
func (e *OcEnv) Delete() {
	fmt.Printf("Cleaning up OpenShift environment %s\n", e.Options.Positional.Alias)
	os.RemoveAll(e.Path)
}

func (e *OcEnv) ensureEnvDir() {
	if _, err := os.Stat(e.Path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(e.Path, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	e.Exists = true
}
func (e *OcEnv) ensureEnvVariables() {
	envContent := `
export KUBECONFIG="` + e.Path + `/kubeconfig.json"
export OCM_CONFIG="` + e.Path + `/ocm.json"
export PATH="` + e.Path + `/bin:` + os.Getenv("PATH") + `"
`
	if e.Options.ClusterId != "" {
		envContent = envContent + "export CLUSTERID=" + e.Options.ClusterId + "\n"
	}
	direnvfile, err := os.Create(e.Path + "/.ocenv")
	if err != nil {
		log.Fatal(err)
	}
	_, err = direnvfile.WriteString(envContent)
	if err != nil {
		log.Fatal(err)
	}
	defer direnvfile.Close()
}

func (e *OcEnv) createBins() {
	if _, err := os.Stat(e.binPath()); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(e.binPath(), os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}
	e.createBin("oct", "ocm tunnel "+e.Options.ClusterId)
	e.createBin("ocl", "ocm cluster login --token "+e.Options.ClusterId)
	e.createBin("ocd", "ocm describe cluster "+e.Options.ClusterId)
	ocb := `
#!/bin/bash

set -euo pipefail

sudo ls`
	if e.Options.LoginScript != "" {
		ocb += `
while true; do
  sleep 30s
  ` + e.Options.LoginScript + `
done &
` + e.Options.LoginScript + `
echo $! >> .killpids
`
		ocb += `
ocm-backplane tunnel ` + e.Options.ClusterId + ` &
echo $! >> .killpids
sleep 5s
ocm backplane login ` + e.Options.ClusterId + `
`
	}
	e.createBin("ocb", ocb)
}

func (e *OcEnv) createBin(cmd, content string) {
	filepath := e.binPath() + "/" + cmd
	scriptfile := e.ensureFile(filepath)
	defer scriptfile.Close()
	scriptfile.WriteString(content)
	err := os.Chmod(filepath, 0744)
	if err != nil {
		log.Fatalf("Can't update permissions on file %s: %v", filepath, err)
	}
}

func (e *OcEnv) ensureFile(filename string) (file *os.File) {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		file, err = os.Create(filename)
		if err != nil {
			log.Fatalf("Can't create file %s: %v", filename, err)
		}
	}
	return
}

func (e *OcEnv) binPath() string {
	return e.Path + "/bin"
}

func complete() {
	if _, ok := os.LookupEnv("COMP_LINE"); !ok {
		return
	}

	if len(os.Args) < 3 {
		os.Exit(1)
	}

	partialWord := os.Args[2]
	preceedingWord := ""
	if len(os.Args) > 3 {
		preceedingWord = os.Args[3]
	}

	if strings.HasPrefix(partialWord, "-") {
		optionsType := reflect.TypeOf(&Options{})
		for i := 0; i < optionsType.Elem().NumField(); i++ {
			short := "-" + optionsType.Elem().Field(i).Tag.Get("short")
			long := "--" + optionsType.Elem().Field(i).Tag.Get("long")

			if strings.HasPrefix(long, partialWord) {
				fmt.Println(long)
			}
			if strings.HasPrefix(short, partialWord) {
				fmt.Println(short)
			}
		}
		os.Exit(0)

	}

	// can't complete cluster IDs (yet?)
	if preceedingWord == "-c" {
		os.Exit(0)
	}

	files, err := os.ReadDir(os.Getenv("HOME") + "/ocenv/")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		if f.IsDir() && strings.HasPrefix(f.Name(), partialWord) {
			fmt.Println(f.Name())
		}
	}

	os.Exit(0)
}
