package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	providerv1 "github.com/arcoloom/arco-proto/gen/go/arcoloom/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
	grpcserver "github.com/arcoloom/arco-provider-aws/internal/transport/grpc"
	"google.golang.org/grpc"
)

const defaultListenAddress = "127.0.0.1:0"

type StartupInfo struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Token    string `json:"token"`
	PID      int    `json:"pid"`
}

type Server struct {
	listenAddress string
	logger        *slog.Logger
	service       provider.Service
	token         string
}

func NewServer(logger *slog.Logger, service provider.Service) (*Server, error) {
	token, err := newSessionToken()
	if err != nil {
		return nil, err
	}

	return &Server{
		listenAddress: defaultListenAddress,
		logger:        logger,
		service:       service,
		token:         token,
	}, nil
}

func (s *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.listenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	startupInfo, err := s.startupInfo(listener)
	if err != nil {
		return err
	}

	if err := writeStartupInfo(os.Stdout, startupInfo); err != nil {
		return err
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcserver.UnaryServerAuthInterceptor(s.token)),
	)
	providerv1.RegisterProviderServiceServer(grpcServer, grpcserver.NewServer(s.logger, s.service))

	s.logger.Info("grpc server started", "address", startupInfo.Address, "port", startupInfo.Port)

	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("serve grpc: %w", err)
	}

	return nil
}

func (s *Server) startupInfo(listener net.Listener) (StartupInfo, error) {
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return StartupInfo{}, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}

	return StartupInfo{
		Protocol: "grpc",
		Address:  listener.Addr().String(),
		Port:     tcpAddress.Port,
		Token:    s.token,
		PID:      os.Getpid(),
	}, nil
}

func writeStartupInfo(writer io.Writer, info StartupInfo) error {
	encoder := json.NewEncoder(writer)
	if err := encoder.Encode(info); err != nil {
		return fmt.Errorf("encode startup info: %w", err)
	}

	return nil
}

func newSessionToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	return hex.EncodeToString(buffer), nil
}
