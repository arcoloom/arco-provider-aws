package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type Process struct {
	cmd         *exec.Cmd
	startupInfo StartupInfo
}

func Launch(ctx context.Context, binaryPath string, stderr io.Writer, args ...string) (*Process, error) {
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Stderr = stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start provider process: %w", err)
	}

	startupInfo, err := readStartupInfo(stdout)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	return &Process{
		cmd:         cmd,
		startupInfo: startupInfo,
	}, nil
}

func LaunchFromEnvironment(ctx context.Context, binaryPath string, args ...string) (*Process, error) {
	return Launch(ctx, binaryPath, os.Stderr, args...)
}

func (p *Process) StartupInfo() StartupInfo {
	return p.startupInfo
}

func (p *Process) Wait() error {
	return p.cmd.Wait()
}

func (p *Process) Stop() error {
	if p.cmd.Process == nil {
		return nil
	}

	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("kill provider process: %w", err)
	}

	_, err := p.cmd.Process.Wait()
	return err
}

func readStartupInfo(reader io.Reader) (StartupInfo, error) {
	line, err := bufio.NewReader(reader).ReadBytes('\n')
	if err != nil {
		return StartupInfo{}, fmt.Errorf("read startup info: %w", err)
	}

	var info StartupInfo
	if err := json.Unmarshal(line, &info); err != nil {
		return StartupInfo{}, fmt.Errorf("decode startup info: %w", err)
	}

	if info.Port == 0 || info.Token == "" || info.Address == "" {
		return StartupInfo{}, fmt.Errorf("startup info is incomplete")
	}

	return info, nil
}
