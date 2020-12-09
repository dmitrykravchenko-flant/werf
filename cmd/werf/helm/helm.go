package helm

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/werf/werf/pkg/werf"

	"github.com/werf/werf/pkg/deploy/werf_chart"

	helm_secret_decrypt "github.com/werf/werf/cmd/werf/helm/secret/decrypt"
	helm_secret_encrypt "github.com/werf/werf/cmd/werf/helm/secret/encrypt"
	helm_secret_file_decrypt "github.com/werf/werf/cmd/werf/helm/secret/file/decrypt"
	helm_secret_file_edit "github.com/werf/werf/cmd/werf/helm/secret/file/edit"
	helm_secret_file_encrypt "github.com/werf/werf/cmd/werf/helm/secret/file/encrypt"
	helm_secret_generate_secret_key "github.com/werf/werf/cmd/werf/helm/secret/generate_secret_key"
	helm_secret_rotate_secret_key "github.com/werf/werf/cmd/werf/helm/secret/rotate_secret_key"
	helm_secret_values_decrypt "github.com/werf/werf/cmd/werf/helm/secret/values/decrypt"
	helm_secret_values_edit "github.com/werf/werf/cmd/werf/helm/secret/values/edit"
	helm_secret_values_encrypt "github.com/werf/werf/cmd/werf/helm/secret/values/encrypt"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/werf/pkg/deploy/helm"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	"github.com/werf/werf/cmd/werf/common"
	cmd_werf_common "github.com/werf/werf/cmd/werf/common"

	"github.com/spf13/cobra"

	cmd_helm "helm.sh/helm/v3/cmd/helm"
)

var _commonCmdData cmd_werf_common.CmdData

func NewCmd() *cobra.Command {
	ctx := common.BackgroundContext()

	var namespace string
	actionConfig := new(action.Configuration)

	cmd := &cobra.Command{
		Use:   "helm",
		Short: "Manage application deployment with helm",
	}

	wc := werf_chart.NewWerfChart(ctx, nil, "", werf_chart.WerfChartOptions{})

	loader.GlobalLoadOptions = &loader.LoadOptions{
		ChartExtender: wc,
		SubchartExtenderFactoryFunc: func() chart.ChartExtender {
			return werf_chart.NewWerfChart(ctx, nil, "", werf_chart.WerfChartOptions{})
		},
	}

	os.Setenv("HELM_EXPERIMENTAL_OCI", "1")

	cmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", *cmd_helm.Settings.GetNamespaceP(), "namespace scope for this request")
	cmd_werf_common.SetupTmpDir(&_commonCmdData, cmd)
	cmd_werf_common.SetupHomeDir(&_commonCmdData, cmd)
	cmd_werf_common.SetupKubeConfig(&_commonCmdData, cmd)
	cmd_werf_common.SetupKubeConfigBase64(&_commonCmdData, cmd)
	cmd_werf_common.SetupKubeContext(&_commonCmdData, cmd)
	cmd_werf_common.SetupStatusProgressPeriod(&_commonCmdData, cmd)
	cmd_werf_common.SetupHooksStatusProgressPeriod(&_commonCmdData, cmd)
	cmd_werf_common.SetupReleasesHistoryMax(&_commonCmdData, cmd)
	cmd_werf_common.SetupLogOptions(&_commonCmdData, cmd)

	cmd.AddCommand(
		cmd_helm.NewUninstallCmd(actionConfig, os.Stdout, cmd_helm.UninstallCmdOptions{}),
		cmd_helm.NewDependencyCmd(os.Stdout),
		cmd_helm.NewGetCmd(actionConfig, os.Stdout),
		cmd_helm.NewHistoryCmd(actionConfig, os.Stdout),
		cmd_helm.NewLintCmd(os.Stdout),
		cmd_helm.NewListCmd(actionConfig, os.Stdout),
		NewTemplateCmd(actionConfig, wc),
		cmd_helm.NewRepoCmd(os.Stdout),
		cmd_helm.NewRollbackCmd(actionConfig, os.Stdout),
		NewInstallCmd(actionConfig, wc),
		NewUpgradeCmd(actionConfig, wc),
		cmd_helm.NewCreateCmd(os.Stdout),
		cmd_helm.NewEnvCmd(os.Stdout),
		cmd_helm.NewPackageCmd(os.Stdout),
		cmd_helm.NewPluginCmd(os.Stdout),
		cmd_helm.NewPullCmd(os.Stdout),
		cmd_helm.NewSearchCmd(os.Stdout),
		cmd_helm.NewShowCmd(os.Stdout),
		cmd_helm.NewStatusCmd(actionConfig, os.Stdout),
		cmd_helm.NewTestCmd(actionConfig, os.Stdout),
		cmd_helm.NewVerifyCmd(os.Stdout),
		cmd_helm.NewVersionCmd(os.Stdout),
		cmd_helm.NewChartCmd(actionConfig, os.Stdout),
		secretCmd(),
		NewGetAutogeneratedValuesCmd(),
		NewGetNamespaceCmd(),
		NewGetReleaseCmd(),
	)

	cmd_helm.LoadPlugins(cmd, os.Stdout)

	commandsQueue := []*cobra.Command{cmd}
	for len(commandsQueue) > 0 {
		cmd := commandsQueue[0]
		commandsQueue = commandsQueue[1:]

		for _, cmd := range cmd.Commands() {
			commandsQueue = append(commandsQueue, cmd)
		}

		if cmd.Runnable() {
			oldRunE := cmd.RunE
			oldRun := cmd.Run
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				// NOTE: Common init block for all runnable commands.

				if err := werf.Init(*_commonCmdData.TmpDir, *_commonCmdData.HomeDir); err != nil {
					return err
				}

				if err := common.ProcessLogOptions(&_commonCmdData); err != nil {
					common.PrintHelp(cmd)
					return err
				}

				// FIXME: setup namespace env var for helm diff plugin
				os.Setenv("WERF_HELM3_MODE", "1")

				ctx := common.BackgroundContext()

				if vals, err := werf_chart.GetServiceValues(ctx, "PROJECT", "REPO", namespace, nil, werf_chart.ServiceValuesOptions{IsStub: true}); err != nil {
					return fmt.Errorf("error creating service values: %s", err)
				} else if err := wc.SetServiceValues(vals); err != nil {
					return err
				}

				helm.InitActionConfig(ctx, namespace, cmd_helm.Settings, actionConfig, helm.InitActionConfigOptions{
					StatusProgressPeriod:      time.Duration(*_commonCmdData.StatusProgressPeriodSeconds) * time.Second,
					HooksStatusProgressPeriod: time.Duration(*_commonCmdData.HooksStatusProgressPeriodSeconds) * time.Second,
					KubeConfigOptions: kube.KubeConfigOptions{
						Context:          *_commonCmdData.KubeContext,
						ConfigPath:       *_commonCmdData.KubeConfig,
						ConfigDataBase64: *_commonCmdData.KubeConfigBase64,
					},
					ReleasesHistoryMax: *_commonCmdData.ReleasesHistoryMax,
				})

				// FIXME: not all `werf helm *` commands may need a kubernetes connection
				if err := kube.Init(kube.InitOptions{KubeConfigOptions: kube.KubeConfigOptions{
					Context:          *_commonCmdData.KubeContext,
					ConfigPath:       *_commonCmdData.KubeConfig,
					ConfigDataBase64: *_commonCmdData.KubeConfigBase64,
				}}); err != nil {
					return fmt.Errorf("cannot initialize kube: %s", err)
				}

				if err := common.InitKubedog(ctx); err != nil {
					return fmt.Errorf("cannot init kubedog: %s", err)
				}

				if oldRun != nil {
					oldRun(cmd, args)
					return nil
				} else {
					if err := oldRunE(cmd, args); err != nil {
						errValue := reflect.ValueOf(err)
						if errValue.Kind() == reflect.Struct {
							if !errValue.IsZero() {
								codeValue := errValue.FieldByName("code")
								if codeValue.IsValid() && !codeValue.IsZero() {
									os.Exit(int(codeValue.Int()))
								}
							}
						}

						return err
					}

					return nil
				}
			}
		}
	}

	return cmd
}

func secretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Work with secrets",
	}

	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Work with secret files",
	}

	fileCmd.AddCommand(
		helm_secret_file_encrypt.NewCmd(),
		helm_secret_file_decrypt.NewCmd(),
		helm_secret_file_edit.NewCmd(),
	)

	valuesCmd := &cobra.Command{
		Use:   "values",
		Short: "Work with secret values files",
	}

	valuesCmd.AddCommand(
		helm_secret_values_encrypt.NewCmd(),
		helm_secret_values_decrypt.NewCmd(),
		helm_secret_values_edit.NewCmd(),
	)

	cmd.AddCommand(
		fileCmd,
		valuesCmd,
		helm_secret_generate_secret_key.NewCmd(),
		helm_secret_encrypt.NewCmd(),
		helm_secret_decrypt.NewCmd(),
		helm_secret_rotate_secret_key.NewCmd(),
	)

	return cmd
}
