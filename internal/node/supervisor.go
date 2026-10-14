package node

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type Engine interface {
	Check(context.Context, string) error
	Start(string) error
	Stop() error
	Running() bool
}

// Process never executes a shell or exposes sing-box output (which may contain secrets).
type Process struct {
	Binary string
	mu     sync.Mutex
	cmd    *exec.Cmd
	done   chan struct{}
}

func (p *Process) Check(ctx context.Context, path string) error {
	if exec.CommandContext(ctx, p.Binary, "check", "-c", path).Run() != nil {
		return errors.New("sing-box validation failed; check supported version and TLS files")
	}
	return nil
}
func (p *Process) Start(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cmd := exec.Command(p.Binary, "run", "-c", path)
	if cmd.Start() != nil {
		return errors.New("sing-box could not start")
	}
	p.cmd = cmd
	p.done = make(chan struct{})
	done := p.done
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return errors.New("sing-box exited during startup")
	case <-time.After(750 * time.Millisecond):
		return nil
	}
}
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return nil
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	return nil
}
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

type Status struct {
	Running         bool      `json:"running"`
	Hash            string    `json:"hash"`
	CertificateHash string    `json:"certificate_hash"`
	TLSReady        bool      `json:"tls_ready"`
	LastSuccess     time.Time `json:"last_success"`
	Error           string    `json:"error,omitempty"`
}
type Supervisor struct {
	mu          sync.Mutex
	Dir         string
	Engine      Engine
	status      Status
	Certificate func(Spec) (string, error)
}

func New(dir string, e Engine) *Supervisor {
	return &Supervisor{Dir: dir, Engine: e, Certificate: func(s Spec) (string, error) {
		t := s.Inbounds[0].TLS
		return CertificateFingerprint(t.CertificatePath, t.KeyPath, t.ServerName)
	}}
}
func AtomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Supervisor) Apply(ctx context.Context, spec Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.apply(ctx, spec)
}
func (s *Supervisor) apply(ctx context.Context, spec Spec) error {
	fail := func(message string) error { s.status.Error = message; return errors.New(message) }
	if err := spec.Validate(); err != nil {
		return fail("node config rejected")
	}
	certHash, err := s.Certificate(spec)
	if err != nil {
		return fail("TLS certificate unavailable, expired or SNI mismatch")
	}
	payload, err := Encode(spec)
	if err != nil {
		return fail("invalid node JSON")
	}
	h := Hash(payload)
	if s.status.Hash == h && s.status.CertificateHash == certHash && s.Engine.Running() {
		s.status.Error = ""
		return nil
	}
	path := filepath.Join(s.Dir, "config.json")
	pending := filepath.Join(s.Dir, "check.json")
	if AtomicWrite(pending, payload) != nil {
		return fail("cannot write candidate config")
	}
	defer os.Remove(pending)
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if s.Engine.Check(checkCtx, pending) != nil {
		return fail("sing-box config check failed")
	}
	previous, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fail("cannot read previous config")
	}
	if len(previous) > 0 && AtomicWrite(filepath.Join(s.Dir, "previous.json"), previous) != nil {
		return fail("cannot back up previous config")
	}
	if AtomicWrite(path, payload) != nil {
		return fail("cannot replace config")
	}
	if s.Engine.Stop() != nil {
		_ = AtomicWrite(path, previous)
		return fail("cannot stop sing-box")
	}
	if s.Engine.Start(path) != nil || !s.Engine.Running() {
		_ = s.Engine.Stop()
		if len(previous) > 0 {
			if AtomicWrite(path, previous) != nil || s.Engine.Start(path) != nil {
				return fail("apply and rollback failed")
			}
		} else {
			_ = os.Remove(path)
		}
		return fail("apply failed; previous config restored")
	}
	s.status = Status{Running: true, Hash: h, CertificateHash: certHash, TLSReady: true, LastSuccess: time.Now().UTC()}
	state, _ := json.Marshal(s.status)
	if AtomicWrite(filepath.Join(s.Dir, "status.json"), state) != nil {
		return fail("config applied but sync state cannot be persisted")
	}
	return nil
}
func (s *Supervisor) Restore(ctx context.Context) error {
	b, e := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return errors.New("cannot read persisted config")
	}
	var spec Spec
	if json.Unmarshal(b, &spec) != nil {
		return errors.New("invalid persisted config")
	}
	return s.Apply(ctx, spec)
}
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.status
	r.Running = s.Engine.Running()
	b, e := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	var spec Spec
	if e == nil && json.Unmarshal(b, &spec) == nil && spec.Validate() == nil {
		h, e := s.Certificate(spec)
		r.TLSReady = e == nil && h == r.CertificateHash
	} else {
		r.TLSReady = false
	}
	return r
}
func (s *Supervisor) Close() { s.mu.Lock(); defer s.mu.Unlock(); _ = s.Engine.Stop() }
