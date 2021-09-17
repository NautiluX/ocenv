package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

type Options struct {
	ClusterId        string
	Alias            string
	Environment      string
	LoginScript      string
	ExportKubeConfig bool
	ResetEnv         bool
	TempEnv          bool
	DeleteEnv        bool
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
	flag.Usage = usage
	flag.StringVar(&options.ClusterId, "c", "", "Cluster ID")
	flag.StringVar(&options.LoginScript, "l", "", "OCM login script to execute in a loop in ocb every 30 seconds")
	flag.BoolVar(&options.ExportKubeConfig, "k", false, "Export KubeConfig")
	flag.BoolVar(&options.ResetEnv, "r", false, "Reset environment")
	flag.BoolVar(&options.TempEnv, "t", false, "Delete environment on exit")
	flag.BoolVar(&options.DeleteEnv, "d", false, "Delete environment")
	flag.Parse()

	if flag.CommandLine.NArg() > 0 {
		options.Alias = flag.Arg(0)
	}

	if options.ClusterId == "" && options.Alias == "" {
		flag.Usage()
		log.Fatal("ClusterId or Alias required")
	}

	if options.Alias == "" {
		log.Println("No Alias set, using cluster ID")
		options.Alias = options.ClusterId
	}

	env := OcEnv{
		Path:    os.Getenv("HOME") + "/ocenv/" + options.Alias,
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
	env.Start()
	if options.TempEnv {
		env.Delete()
	}

}

func usage() {
	log.Printf("Usage: %s [OPTIONS] [<Alias>]\nEither <Alias> or <Cluster ID> must be passed.\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
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
		e.allowDirenv()
	}
}

func (e *OcEnv) PrintKubeConfigExport() {
	fmt.Printf("export KUBECONFIG=\"%s\"\n", e.Path+"/kubeconfig.json")
}

func (e *OcEnv) Start() {
	me, err := user.Current()
	if err != nil {
		panic(err)
	}

	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   e.Path,
	}

	fmt.Print("Switching to OpenShift environment " + e.Options.Alias + "\n")
	proc, err := os.StartProcess("/usr/bin/login", []string{"login", "-fpl", me.Username}, &pa)
	if err != nil {
		panic(err)
	}

	state, err := proc.Wait()
	if err != nil {
		panic(err)
	}

	e.killChilds()

	fmt.Printf("Exited OpenShift environment %s\n", state.String())

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
	fmt.Printf("Cleaning up OpenShift environment %s\n", e.Options.Alias)
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
export KUBECONFIG="$(pwd)/kubeconfig.json"
export OCM_CONFIG="$(pwd)/ocm.json"
PATH_add ` + e.binPath() + `
`
	if e.Options.ClusterId != "" {
		envContent = envContent + "export CLUSTERID=\"" + e.Options.ClusterId + "\"\n"
	}
	direnvfile, err := os.Create(e.Path + "/.envrc")
	if err != nil {
		log.Fatal(err)
	}
	_, err = direnvfile.WriteString(envContent)
	if err != nil {
		log.Fatal(err)
	}
	defer direnvfile.Close()
}

func (e *OcEnv) allowDirenv() {
	cmd := exec.Command("direnv", "allow")
	cmd.Dir = e.Path
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Running `direnv` finished with error: %v", err)
	}
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
