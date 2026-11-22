package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"flypanel/internal/auth"
	"flypanel/internal/config"
	"flypanel/internal/store"
)

// adminCommand dispatches `flypanel admin init|status`.
func adminCommand(args []string) {
	if len(args) == 0 {
		adminUsage()
	}
	switch args[0] {
	case "init":
		adminInit(args[1:])
	case "status":
		adminStatusCommand(args[1:])
	default:
		adminUsage()
	}
}

func adminUsage() {
	fmt.Fprintln(os.Stderr, "usage: flypanel admin init [--data-dir DIR] [--password-stdin] [username]")
	fmt.Fprintln(os.Stderr, "       flypanel admin status [--data-dir DIR]")
	os.Exit(2)
}

// adminStatusCommand reports whether an admin account exists: exit 0 for
// "exists", 1 for "none", so the installer can skip the init handoff.
func adminStatusCommand(args []string) {
	fs := flag.NewFlagSet("admin status", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "data directory (default: DATA_DIR from the env file)")
	fs.Parse(args)

	dir, err := resolveDataDir(*dataDir)
	if err != nil {
		fatalf("admin status: %v", err)
	}
	exists, err := adminStatus(dir)
	if err != nil {
		fatalf("admin status: %v", err)
	}
	if !exists {
		fmt.Println("no admin")
		os.Exit(1)
	}
	fmt.Println("admin exists")
}

func adminStatus(dir string) (bool, error) {
	st, err := store.Open(dir)
	if err != nil {
		return false, err
	}
	defer st.Close()
	if _, err := st.AdminGet(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// resolveDataDir prefers --data-dir, then DATA_DIR from the env file.
func resolveDataDir(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.DataDir, nil
}

// adminInit implements `flypanel admin init`: interactive username and
// password prompts (or --password-stdin for automation) creating the single
// admin account.
func adminInit(args []string) {
	fs := flag.NewFlagSet("admin init", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "data directory (default: DATA_DIR from the env file)")
	passwordStdin := fs.Bool("password-stdin", false, "read the password from stdin without confirmation")
	fs.Parse(args)

	username := ""
	if fs.NArg() > 0 {
		username = fs.Arg(0)
	}

	dir, err := resolveDataDir(*dataDir)
	if err != nil {
		fatalf("admin init: %v", err)
	}

	if username == "" {
		username, err = promptLine("admin username: ")
		if err != nil {
			fatalf("admin init: %v", err)
		}
	}

	password, err := readPassword(*passwordStdin)
	if err != nil {
		fatalf("admin init: %v", err)
	}

	st, err := store.Open(dir)
	if err != nil {
		fatalf("admin init: %v", err)
	}
	defer st.Close()

	if err := auth.AdminInit(st, username, password); err != nil {
		fatalf("admin init: %v", err)
	}
	fmt.Println("admin account created")
}

// readPassword reads the password from stdin when --password-stdin is set;
// otherwise it prompts twice via the terminal with confirmation.
func readPassword(stdin bool) (string, error) {
	if stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "admin password (min 12 chars): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(pw) != string(confirm) {
		return "", errors.New("passwords do not match")
	}
	return string(pw), nil
}

func promptLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
