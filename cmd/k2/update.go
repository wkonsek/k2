package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const installURL = "https://raw.githubusercontent.com/wkonsek/k2/main/install.sh"

type updater struct {
	url    string
	client *http.Client
	bash   func() (string, error)
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func isUpdate(args []string) bool {
	return len(args) > 0 && (args[0] == "--update" || args[0] == "-U")
}

func runUpdate(args []string) int {
	return updater{
		url:    installURL,
		client: &http.Client{Timeout: 30 * time.Second},
		bash:   func() (string, error) { return exec.LookPath("bash") },
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}.run(args)
}

func (u updater) run(args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(u.stderr, "k2: %s takes no arguments (set K2_VERSION=<tag> to pick a release)\n", args[0])
		return 2
	}
	bash, err := u.bash()
	if err != nil {
		fmt.Fprintln(u.stderr, "k2: bash is required to update (install bash, or use a package from the releases page)")
		return 1
	}
	file, err := u.fetch()
	if err != nil {
		fmt.Fprintf(u.stderr, "k2: cannot download the install script: %v\n", err)
		return 1
	}
	defer os.Remove(file)
	cmd := exec.Command(bash, file)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = u.stdin, u.stdout, u.stderr
	err = cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() >= 0 {
		return ee.ExitCode()
	}
	fmt.Fprintf(u.stderr, "k2: cannot run the install script: %v\n", err)
	return 1
}

func (u updater) fetch() (string, error) {
	resp, err := u.client.Get(u.url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %s", u.url, resp.Status)
	}
	f, err := os.CreateTemp("", "k2-install-*.sh")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 1<<20)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
