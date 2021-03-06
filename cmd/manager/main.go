package main

import (
	"flag"
	golog "log"
	"math/rand"
	"os"
	"time"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	crv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"k8s.io/klog"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	"github.com/openshift/hive/pkg/apis"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel             = "info"
	leaderElectionConfigMap     = "hive-controllers-leader"
	leaderElectionLeaseDuration = "40s"
	leaderElectionRenewDeadline = "20s"
	leaderElectionlRetryPeriod  = "4s"
)

type controllerManagerOptions struct {
	LogLevel string
}

func newRootCommand() *cobra.Command {
	opts := &controllerManagerOptions{}
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "OpenShift Hive controller manager.",
		Run: func(cmd *cobra.Command, args []string) {
			// Set log level
			level, err := log.ParseLevel(opts.LogLevel)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse log level")
			}
			log.SetLevel(level)
			log.Infof("Version: %s @ %s", version.String, version.Commit)
			log.Debug("debug logging enabled")

			// Parse leader election options
			leaseDuration, err := time.ParseDuration(leaderElectionLeaseDuration)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse lease duration")
			}
			renewDeadline, err := time.ParseDuration(leaderElectionRenewDeadline)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse renew deadline")
			}
			retryPeriod, err := time.ParseDuration(leaderElectionlRetryPeriod)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse retry period")
			}

			// Get a config to talk to the apiserver
			cfg, err := config.GetConfig()
			if err != nil {
				log.Fatal(err)
			}

			if os.Getenv(constants.HiveNamespaceEnvVar) == "" {
				log.Warnf("%s env var is not defined, using default: %s", constants.HiveNamespaceEnvVar,
					constants.DefaultHiveNamespace)
			}
			hiveNSName := utils.GetHiveNamespace()
			log.Infof("hive namespace: %s", hiveNSName)

			// Create a new Cmd to provide shared dependencies and start components
			mgr, err := manager.New(cfg, manager.Options{
				LeaderElection:          true,
				LeaderElectionNamespace: hiveNSName,
				LeaderElectionID:        leaderElectionConfigMap,
				LeaseDuration:           &leaseDuration,
				RenewDeadline:           &renewDeadline,
				RetryPeriod:             &retryPeriod,
				MetricsBindAddress:      ":2112",
				HealthProbeBindAddress:  ":8080",
			})
			if err != nil {
				log.Fatal(err)
			}

			log.Info("Registering Components.")

			if err := utils.SetupAdditionalCA(); err != nil {
				log.Fatal(err)
			}

			// Setup Scheme for all resources
			if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := openshiftapiv1.Install(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := apiextv1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := crv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := velerov1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := awsprovider.SchemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			// Setup all Controllers
			if err := controller.AddToManager(mgr); err != nil {
				log.Fatal(err)
			}

			mgr.AddReadyzCheck("ping", healthz.Ping)
			mgr.AddHealthzCheck("ping", healthz.Ping)

			log.Info("Starting the Cmd.")

			// Start the Cmd
			log.Fatal(mgr.Start(signals.SetupSignalHandler()))
		},
	}

	cmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", defaultLogLevel, "Log level (debug,info,warn,error,fatal)")
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	initializeKlog(cmd.PersistentFlags())
	flag.CommandLine.Parse([]string{})

	return cmd
}

func initializeKlog(flags *pflag.FlagSet) {
	golog.SetOutput(klogWriter{}) // Redirect all regular go log output to klog
	golog.SetFlags(0)

	go wait.Forever(klog.Flush, 5*time.Second) // Periodically flush logs
	f := flags.Lookup("logtostderr")           // Default to logging to stderr
	if f != nil {
		f.Value.Set("true")
	}
}

type klogWriter struct{}

func (writer klogWriter) Write(data []byte) (n int, err error) {
	klog.Info(string(data))
	return len(data), nil
}

func main() {
	defer klog.Flush()
	rand.Seed(time.Now().UnixNano())
	cmd := newRootCommand()
	err := cmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
