package extensions

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/sausheong/harness/execution"
)

// ContainerLaunch is part of the launch approval digest. Runtime libraries and
// interpreters must be supplied by the immutable image or reviewed resources.
type ContainerLaunch struct {
	Docker   string `json:"docker"`
	Socket   string `json:"socket"`
	Image    string `json:"image"`
	Writable bool   `json:"writable"`
	Network  bool   `json:"network"`
}

func validateContainerLaunch(c ContainerLaunch) error {
	if !filepath.IsAbs(c.Docker) || !filepath.IsAbs(c.Socket) || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(c.Image) {
		return errors.New("container launch requires absolute Docker/socket paths and immutable image ID")
	}
	return nil
}
func launchBoundary(c *ContainerLaunch) string {
	if c == nil {
		return "unrestricted host subprocess; capabilities are not an OS sandbox; runtime libraries are not snapshotted"
	}
	return fmt.Sprintf("container image=%s; workspace writable=%t; network=%t; reviewed resources read-only; other host paths unmounted", c.Image, c.Writable, c.Network)
}
func ReviewContainerLaunch(ctx context.Context, cfg LaunchConfig, boundary ContainerLaunch) (LaunchReview, error) {
	if err := validateContainerLaunch(boundary); err != nil {
		return LaunchReview{}, err
	}
	r, err := reviewLaunchFiles(ctx, cfg)
	if err != nil {
		return r, err
	}
	r.Container = &boundary
	r.Boundary = launchBoundary(&boundary)
	r.Specification.Digest, err = reviewDigest(r)
	return r, err
}
func NewContainerFactory(reviews []LaunchReview, admit LaunchAdmission) (Factory, error) {
	if admit == nil || len(reviews) > 16 {
		return nil, errors.New("explicit container admission required")
	}
	byName := map[string]LaunchReview{}
	for _, input := range reviews {
		r := cloneReview(input)
		if r.Container == nil {
			return nil, errors.New("host review cannot authorise container launch")
		}
		if err := validateLaunchReview(r); err != nil {
			return nil, err
		}
		digest, err := reviewDigest(r)
		if err != nil || digest != r.Specification.Digest {
			return nil, errors.New("container launch digest mismatch")
		}
		normalized, err := normalizeSpecifications([]Specification{r.Specification})
		if err != nil || !reflect.DeepEqual(normalized[0], r.Specification) {
			return nil, errors.New("invalid container specification")
		}
		if _, exists := byName[r.Specification.Name]; exists {
			return nil, errors.New("duplicate container review")
		}
		byName[r.Specification.Name] = r
	}
	return func(operation, lifetime context.Context, spec Specification) (*Connection, error) {
		r, ok := byName[spec.Name]
		if !ok || !reflect.DeepEqual(spec, r.Specification) {
			return nil, errors.New("container extension launch not reviewed")
		}
		if err := operation.Err(); err != nil {
			return nil, err
		}
		if err := admit(operation, cloneReview(r)); err != nil {
			return nil, err
		}
		cfg := r.Container
		backend := execution.Container{Docker: cfg.Docker, Socket: cfg.Socket, Image: cfg.Image, Workspace: r.Workspace, Writable: cfg.Writable, Network: cfg.Network}
		backend.Resources = append(backend.Resources, execution.ReadOnlyResource{Path: r.Binary.Path, Relative: "executable", SHA256: r.Binary.SHA256, Executable: len(r.ImageInterpreter) == 0})
		for _, asset := range r.Assets {
			backend.Resources = append(backend.Resources, execution.ReadOnlyResource{Path: asset.Path, Relative: "package/" + asset.Relative, SHA256: asset.SHA256, Executable: asset.Mode&0111 != 0})
		}
		args := append(append([]string(nil), r.ImageInterpreter...), "/harness-resources/executable")
		for _, arg := range r.Arguments {
			if strings.HasPrefix(arg, "${package}/") {
				arg = "/harness-resources/package/" + strings.TrimPrefix(arg, "${package}/")
			}
			args = append(args, arg)
		}
		owned, cancel := context.WithCancel(lifetime)
		stopOperation := context.AfterFunc(operation, cancel)
		stream, err := backend.OpenStream(owned, execution.Request{Argv: args, Env: r.Environment})
		stopOperation()
		if err != nil {
			cancel()
			return nil, err
		}
		if err = operation.Err(); err != nil {
			cancel()
			return nil, errors.Join(err, stream.Close())
		}
		transport := Transport{Input: stream.Stdin, Output: stream.Stdout, Stderr: stream.Stderr, Boundary: r.Boundary, Stop: cancel, Wait: func() error {
			defer cancel()
			err := errors.Join(stream.Wait(), stream.Close())
			if err != nil {
				return fmt.Errorf("%w: %w", ErrTransportCleanup, err)
			}
			return nil
		}}
		connection, err := Connect(lifetime, transport)
		if err != nil {
			cancel()
			return nil, errors.Join(err, stream.Close())
		}
		return connection, nil
	}, nil
}

func validateImageInterpreter(argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	if len(argv) > 16 || !strings.HasPrefix(argv[0], "/") || filepath.ToSlash(filepath.Clean(argv[0])) != argv[0] {
		return errors.New("image interpreter requires a clean absolute image path and at most 16 arguments")
	}
	for _, prefix := range []string{"/workspace", "/tmp", "/harness-resources", "/hand-worker"} {
		if argv[0] == prefix || strings.HasPrefix(argv[0], prefix+"/") {
			return errors.New("image interpreter must come from immutable image filesystem")
		}
	}
	size := 0
	for _, arg := range argv {
		size += len(arg)
		if strings.ContainsRune(arg, 0) {
			return errors.New("invalid image interpreter argument")
		}
	}
	if size > 8192 {
		return errors.New("image interpreter arguments exceed 8 KiB")
	}
	return nil
}
