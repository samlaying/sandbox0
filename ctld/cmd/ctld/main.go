package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	ctldfuseplugin "github.com/sandbox0-ai/sandbox0/ctld/internal/ctld/fuseplugin"
	ctldpower "github.com/sandbox0-ai/sandbox0/ctld/internal/ctld/power"
	ctldserver "github.com/sandbox0-ai/sandbox0/ctld/internal/ctld/server"
	"github.com/sandbox0-ai/sandbox0/pkg/k8s"
	"github.com/sandbox0-ai/sandbox0/pkg/observability"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

var (
	mountsAllowed = 5
	httpAddr      = ":8095"
	kubeconfig    = ""
	cgroupRoot    = "/host-sys/fs/cgroup"
	criEndpoint   = "/host-run/containerd/containerd.sock"
	procRoot      = "/proc"
	nodeName      = os.Getenv("NODE_NAME")
)

func main() {
	flag.IntVar(&mountsAllowed, "mounts-allowed", 100, "maximum times the fuse device can be mounted")
	flag.StringVar(&httpAddr, "http-addr", ":8095", "HTTP listen address for ctld health and control endpoints")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "optional kubeconfig path used by ctld")
	flag.StringVar(&cgroupRoot, "cgroup-root", "/host-sys/fs/cgroup", "host cgroup root mounted into ctld")
	flag.StringVar(&criEndpoint, "cri-endpoint", "/host-run/containerd/containerd.sock", "host CRI socket used to read pod sandbox stats")
	flag.StringVar(&procRoot, "proc-root", "/proc", "host proc root used to inspect sandbox processes")
	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "current node name used to validate local sandbox ownership")
	flag.Parse()

	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	obsProvider, err := observability.New(observability.Config{
		ServiceName: "ctld",
		Logger:      logger,
		TraceExporter: observability.TraceExporterConfig{
			Type:     os.Getenv("OTEL_EXPORTER_TYPE"),
			Endpoint: os.Getenv("OTEL_EXPORTER_ENDPOINT"),
		},
	})
	if err != nil {
		logger.Fatal("Failed to initialize observability", zap.Error(err))
	}
	defer obsProvider.Shutdown(context.Background())

	logger.Info("Starting ctld")
	defer func() { logger.Info("Stopped ctld") }()

	httpServer := newHTTPServerWithObservability(httpAddr, buildPowerController(obsProvider), logger, obsProvider)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("ctld http server failed", zap.Error(err))
		}
	}()

	logger.Info("Starting FS watcher")
	watcher, err := ctldfuseplugin.NewFSWatcher(pluginapi.DevicePluginPath)
	if err != nil {
		logger.Error("Failed to create FS watcher", zap.Error(err))
		os.Exit(1)
	}
	defer watcher.Close()

	logger.Info("Starting OS watcher")
	sigs := ctldfuseplugin.NewOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	restart := true
	var devicePlugin *ctldfuseplugin.DevicePlugin

L:
	for {
		if restart {
			if devicePlugin != nil {
				devicePlugin.Stop()
			}

			devicePlugin = ctldfuseplugin.NewDevicePlugin(mountsAllowed)
			if err := devicePlugin.Serve(); err != nil {
				logger.Warn("Could not contact Kubelet, retrying")
			} else {
				restart = false
			}
		}

		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				logger.Info("Kubelet socket created, restarting", zap.String("socket", pluginapi.KubeletSocket))
				restart = true
			}

		case err := <-watcher.Errors:
			logger.Warn("FS watcher error", zap.Error(err))

		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				logger.Info("Received SIGHUP, restarting")
				restart = true
			default:
				logger.Info("Received shutdown signal", zap.String("signal", s.String()))
				if devicePlugin != nil {
					devicePlugin.Stop()
				}
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = httpServer.Shutdown(shutdownCtx)
				cancel()
				break L
			}
		}
	}
}

func newHTTPServer(addr string, controller ctldserver.Controller) *http.Server {
	return newHTTPServerWithObservability(addr, controller, nil, nil)
}

func newHTTPServerWithObservability(addr string, controller ctldserver.Controller, logger *zap.Logger, obsProvider *observability.Provider) *http.Server {
	return &http.Server{Addr: addr, Handler: ctldserver.NewMuxWithObservability(controller, logger, obsProvider)}
}

func buildPowerController(obsProvider *observability.Provider) ctldserver.Controller {
	var (
		k8sClient kubernetes.Interface
		err       error
	)
	if obsProvider != nil {
		k8sClient, err = k8s.NewClientWithObservability(kubeconfig, obsProvider)
	} else {
		k8sClient, err = k8s.NewClient(kubeconfig)
	}
	if err != nil {
		fmt.Printf("ctld power control disabled: build kubernetes client: %v\n", err)
		return ctldserver.NotImplementedController{}
	}
	resolver := ctldpower.NewPodResolver(k8sClient, nodeName, cgroupRoot)
	resolver.ProcRoot = procRoot
	controller := ctldpower.NewController(resolver, nil)
	controller.StatsProvider = ctldpower.NewCRIStatsProvider(criEndpoint)
	if obsProvider != nil {
		controller.SetMetrics(ctldpower.NewMetrics(obsProvider.MetricsRegistryOrNil()))
	}
	return controller
}

func initLogger() (*zap.Logger, error) {
	cfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapcore.InfoLevel),
		Development: false,
		Encoding:    "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	return cfg.Build()
}
