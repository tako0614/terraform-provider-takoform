// reference-host serves the stable Host API v1 reference implementation over
// HTTP so a person can inspect the exact Edge Family lifecycle behavior.
//
// Everything this serves already existed: the conformance corpus builds this
// exact host in-process to measure itself, and cmd/worker-authoring-conformance
// drives a real OpenTofu or Terraform CLI against it on every `bun run check`.
// What was missing was a way for a reader to do the same thing, so the
// published provider had a documented endpoint of https://forms.example.com and
// nowhere to actually point.
//
// What this is NOT:
//
//   - It is not a production host. It stores desired state and serves no
//     application traffic: a ModuleWorker created here has no isolate, a
//     WorkerEndpoint's address answers nothing, and a queue delivers no message.
//     The lane it implements drives desired state and never moves a byte of
//     application data (spec/host-api/v1.md).
//   - It is not safe to expose. It implements the runner-only
//     Takoform-Conformance-Probe headers, which let a caller force error codes
//     and drive host-side state transitions, and its credentials are three
//     constants compiled into this repository. Bind it to loopback.
//
// It is a host you can learn and develop against, and it is honest about being
// exactly that.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "reference-host:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("reference-host", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	addr := flags.String("addr", "127.0.0.1:8080", "loopback address to listen on")
	repoRoot := flags.String("repo-root", ".", "repository root holding the Form candidate catalog")
	contractPath := flags.String(
		"contract",
		filepath.Join("conformance", "takoform-v1", "generic-host", "portable-host"),
		"portable host contract directory, relative to --repo-root",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		return err
	}
	contract, err := portableconformancev3.Verify(filepath.Join(root, *contractPath))
	if err != nil {
		return fmt.Errorf("verify the portable host contract: %w", err)
	}
	catalog, err := portableconformancev3.LoadCatalog(root, contract)
	if err != nil {
		return fmt.Errorf("load the Edge Platform Family catalog: %w", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	// The banner's "keep it on loopback" is a safety boundary, not advice: this
	// handler implements the runner-only probe headers and its credentials are
	// constants in this repository. Enforce the boundary mechanically rather
	// than stating it — an --addr of 0.0.0.0 or a public interface is refused,
	// whatever the flag said.
	if tcp, ok := listener.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		_ = listener.Close()
		return fmt.Errorf(
			"refusing to listen on %s: this host serves conformance probe headers and "+
				"repository-known credentials, so it binds loopback addresses only",
			listener.Addr(),
		)
	}
	origin := "http://" + listener.Addr().String()

	fmt.Fprintf(stdout, "Takoform reference host listening on %s\n", origin)
	// From the CORPUS, not from the package constants: those name the retained
	// lane, so a host serving the current corpus announced an address that
	// answers 404 and told its operator to point a provider at it.
	fmt.Fprintf(stdout, "  lane      %s\n", contract.Lane())
	fmt.Fprintf(stdout, "  discovery %s%s\n", origin, contract.LaneDiscoveryPath())
	fmt.Fprintf(stdout, "\nPoint a provider at it:\n")
	fmt.Fprintf(stdout, "  export TAKOFORM_ENDPOINT=%s\n", origin)
	fmt.Fprintf(stdout, "  export TAKOFORM_SPACE=dev\n")
	fmt.Fprintf(stdout, "  export TAKOFORM_TOKEN=%s\n", portableconformancev3.ReferencePrimaryToken)
	fmt.Fprintf(stdout, "\nThis host stores desired state and serves no application traffic, and it\n")
	fmt.Fprintf(stdout, "implements the conformance probe headers. Keep it on loopback.\n")

	server := &http.Server{Handler: portableconformancev3.NewReferenceHost(contract, catalog)}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
