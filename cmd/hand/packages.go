package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	extensionprotocol "github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/packages"
)

func runPackages(ctx context.Context, args []string, out io.Writer, handVersion string) error {
	if len(args) == 0 {
		return errors.New("usage: hand packages inspect|show|read|list|review-install|review-archive|review-git|review-remove|review-rollback|review-runtimes|review-extensions|apply [flags]")
	}
	command := args[0]
	allowed := map[string][]string{
		"read":              {"store", "name", "path"},
		"review-runtimes":   {"store", "names", "out", "container"},
		"review-extensions": {"store", "names", "workspace", "snapshots", "runtime-approvals", "out", "container"},
		"inspect":           {"source"}, "list": {"store"}, "show": {"store", "name"},
		"review-install":  {"store", "source", "pin", "out"},
		"review-archive":  {"store", "source", "pin", "archive-sha256", "out"},
		"review-git":      {"store", "source", "pin", "commit", "out"},
		"review-remove":   {"store", "name", "out"},
		"review-rollback": {"store", "name", "pin", "out"},
		"apply":           {"review", "approve"},
	}
	permits, ok := allowed[command]
	if !ok {
		return fmt.Errorf("unknown package command %q", command)
	}
	flags := flag.NewFlagSet("hand packages "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	containerFile := flags.String("container", "", "explicit container boundary and interpreter mapping JSON")
	resourcePath := flags.String("path", "", "inventoried skill or prompt path")
	names := flags.String("names", "", "comma-separated installed package names in launch order")
	workspace := flags.String("workspace", "", "absolute extension workspace")
	snapshots := flags.String("snapshots", "", "absolute private launch snapshot root")
	runtimeApprovals := flags.String("runtime-approvals", "", "JSON document of separately approved package runtime reviews")
	storePath := flags.String("store", "", "private package store (default ~/.hand/packages)")
	source := flags.String("source", "", "explicit local package directory, archive or Git repository")
	archivePin := flags.String("archive-sha256", "", "archive byte SHA-256")
	commit := flags.String("commit", "", "full Git commit SHA-1")
	pin := flags.String("pin", "", "package SHA-256 pin")
	name := flags.String("name", "", "installed package name")
	output := flags.String("out", "", "new review file")
	reviewPath := flags.String("review", "", "review file to apply")
	approval := flags.String("approve", "", "approved package change digest")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected package command arguments")
	}
	var invalid error
	flags.Visit(func(f *flag.Flag) {
		found := false
		for _, allowed := range permits {
			if f.Name == allowed {
				found = true
			}
		}
		if !found {
			invalid = fmt.Errorf("--%s is not valid for %s", f.Name, command)
		}
	})
	if invalid != nil {
		return invalid
	}
	emit := func(value any) error {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	if command == "inspect" {
		if *source == "" {
			return errors.New("--source required")
		}
		m, digest, err := packages.VerifyDirectory(ctx, *source)
		if err != nil {
			return err
		}
		return emit(struct {
			Manifest packages.Manifest `json:"manifest"`
			Digest   string            `json:"package_digest"`
		}{m, digest})
	}
	var selected packages.ChangeReview
	if command == "apply" {
		if *reviewPath == "" || *approval == "" {
			return errors.New("--review and --approve required")
		}
		var err error
		selected, err = readPackageReview(*reviewPath)
		if err != nil {
			return err
		}
		digest, err := selected.ApprovalDigest()
		if err != nil {
			return err
		}
		if digest != *approval {
			return errors.New("package review differs from approved digest")
		}
		*storePath = selected.Store
	} else {
		if command != "list" && command != "show" && command != "read" && *output == "" {
			return errors.New("--out required")
		}
		if (command == "review-install" || command == "review-archive" || command == "review-git") && (*source == "" || *pin == "") {
			return errors.New("--source and --pin required")
		}
		if command == "review-archive" && *archivePin == "" {
			return errors.New("--archive-sha256 required")
		}
		if command == "review-git" && *commit == "" {
			return errors.New("--commit required")
		}
		if (command == "review-remove" || command == "review-rollback" || command == "show" || command == "read") && *name == "" {
			return errors.New("--name required")
		}
		if command == "review-rollback" && *pin == "" {
			return errors.New("--pin required")
		}
	}
	if (command == "review-runtimes" || command == "review-extensions") && *names == "" {
		return errors.New("--names required")
	}
	if command == "review-extensions" && (!filepath.IsAbs(*workspace) || !filepath.IsAbs(*snapshots)) {
		return errors.New("absolute --workspace and --snapshots required")
	}
	if *storePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*storePath = filepath.Join(home, ".hand", "packages")
	}
	store, err := packages.OpenStore(*storePath, handVersion)
	if err != nil {
		return err
	}
	defer store.Close()
	switch command {
	case "read":
		resource, err := store.ReadTextResource(ctx, *name, *resourcePath)
		if err != nil {
			return err
		}
		return emit(resource)
	case "review-runtimes":
		if *containerFile != "" {
			result, err := reviewPackageContainerRuntimes(ctx, store, strings.Split(*names, ","), *containerFile, *output)
			if err != nil {
				return err
			}
			return emit(result)
		}
		selection, err := store.ResolveSelected(ctx, strings.Split(*names, ","))
		if err != nil {
			return err
		}
		reviews := []app.PackageRuntimeApproval{}
		digests := map[string]string{}
		for _, p := range selection.Packages {
			for _, requirement := range p.Manifest.Runtimes {
				review, err := packages.ReviewRuntime(ctx, requirement)
				if err != nil {
					return err
				}
				digest, err := review.Digest()
				if err != nil {
					return err
				}
				digests[p.Name+"/"+requirement.Name] = digest
				reviews = append(reviews, app.PackageRuntimeApproval{Package: p.Name, Review: review})
			}
		}
		filename, err := writePackageReview(*output, packageRuntimeReviews{Reviews: reviews})
		if err != nil {
			return err
		}
		return emit(struct {
			File    string                       `json:"review_file"`
			Reviews []app.PackageRuntimeApproval `json:"reviews"`
			Digests map[string]string            `json:"review_digests"`
		}{filename, reviews, digests})
	case "review-extensions":
		var selected app.ExtensionStartup
		var err error
		if *containerFile != "" {
			selected, err = reviewPackageContainerLaunch(ctx, store, strings.Split(*names, ","), *workspace, *snapshots, *containerFile, *runtimeApprovals)
		} else {
			var approvals packageRuntimeReviews
			if *runtimeApprovals != "" {
				if err = readPackageJSON(*runtimeApprovals, &approvals); err != nil {
					return err
				}
			}
			selected, err = app.ReviewPackageExtensions(ctx, store, strings.Split(*names, ","), *workspace, *snapshots, approvals.Reviews)
		}
		if err != nil {
			return err
		}
		filename, err := writePackageReview(*output, selected)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(selected, "", "  ")
		if err != nil {
			return err
		}
		raw = append(raw, '\n')
		sum := sha256.Sum256(raw)
		return emit(struct {
			File   string               `json:"review_file"`
			Digest string               `json:"approval_digest"`
			Review app.ExtensionStartup `json:"review"`
		}{filename, hex.EncodeToString(sum[:]), selected})
	case "list":
		state, err := store.List()
		if err != nil {
			return err
		}
		return emit(struct {
			Lock     packages.Lockfile       `json:"lock"`
			Recovery packages.RecoveryReport `json:"recovery"`
		}{state, store.Recovery()})
	case "show":
		selected, err := store.ResolveSelected(ctx, []string{*name})
		if err != nil {
			return err
		}
		return emit(selected)
	case "apply":
		result, applyErr := store.Apply(ctx, selected, *approval)
		response := struct {
			Result packages.ChangeResult `json:"result"`
			Error  string                `json:"error,omitempty"`
		}{Result: result}
		if applyErr != nil {
			response.Error = applyErr.Error()
		}
		return errors.Join(applyErr, emit(response))
	case "review-install":
		selected, err = store.PrepareInstall(ctx, *source, *pin)
	case "review-archive", "review-git":
		selected, err = preparePackageImport(ctx, store, command, *source, *pin, *archivePin, *commit)
	case "review-remove":
		selected, err = store.PrepareRemove(*name)
	case "review-rollback":
		selected, err = store.PrepareRollback(ctx, *name, *pin)
	}
	if err != nil {
		return err
	}
	digest, err := selected.ApprovalDigest()
	if err != nil {
		return err
	}
	filename, err := writePackageReview(*output, selected)
	if err != nil {
		return err
	}
	return emit(struct {
		File   string                `json:"review_file"`
		Digest string                `json:"approval_digest"`
		Review packages.ChangeReview `json:"review"`
	}{filename, digest, selected})
}
func readPackageReview(filename string) (packages.ChangeReview, error) {
	var review packages.ChangeReview
	err := readPackageJSON(filename, &review)
	return review, err
}
func readPackageJSON(filename string, target any) error {
	f, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > extensionprotocol.MaxFrameBytes {
		return errors.New("review must be a regular file at most 256 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, extensionprotocol.MaxFrameBytes+1))
	if err != nil {
		return err
	}
	return extensionprotocol.DecodePayload(raw, target)
}
func writePackageReview(filename string, review any) (string, error) {
	raw, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if len(raw) > extensionprotocol.MaxFrameBytes {
		return "", errors.New("review exceeds 256 KiB")
	}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(filename)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	complete = true
	return filename, nil
}

func preparePackageImport(ctx context.Context, store *packages.Store, command, source, pin, archivePin, commit string) (review packages.ChangeReview, err error) {
	parent, err := os.MkdirTemp("", "hand-package-review-")
	if err != nil {
		return review, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(parent)) }()
	var snapshot *packages.Snapshot
	if command == "review-archive" {
		snapshot, err = packages.ImportArchive(ctx, source, parent, archivePin, pin)
	} else {
		snapshot, err = packages.ImportGit(ctx, source, parent, commit, pin)
	}
	if err != nil {
		return review, err
	}
	defer func() { err = errors.Join(err, snapshot.Close()) }()
	return store.PrepareSnapshot(ctx, snapshot)
}

type packageRuntimeReviews struct {
	Reviews []app.PackageRuntimeApproval `json:"reviews"`
}
