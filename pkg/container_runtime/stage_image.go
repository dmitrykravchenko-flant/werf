package container_runtime

import (
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types"

	"github.com/flant/logboek"

	"github.com/flant/shluz"
	"github.com/flant/werf/pkg/docker"
	"github.com/flant/werf/pkg/image"
)

type StageImage struct {
	*baseImage
	fromImage              *StageImage
	container              *StageImageContainer
	buildImage             *buildImage
	dockerfileImageBuilder *DockerfileImageBuilder
}

func NewStageImage(fromImage *StageImage, name string, localDockerServerRuntime *LocalDockerServerRuntime) *StageImage {
	stage := &StageImage{}
	stage.baseImage = newBaseImage(name, localDockerServerRuntime)
	stage.fromImage = fromImage
	stage.container = newStageImageContainer(stage)
	return stage
}

func (i *StageImage) Inspect() *types.ImageInspect {
	return i.inspect
}

func (i *StageImage) BuilderContainer() BuilderContainer {
	return &StageImageBuilderContainer{i}
}

func (i *StageImage) Container() Container {
	return i.container
}

func (i *StageImage) GetID() string {
	if i.buildImage != nil {
		return i.buildImage.Name()
	} else {
		return i.baseImage.GetStagesStorageImageInfo().ID
	}
}

func (i *StageImage) Build(options BuildOptions) error {
	if i.dockerfileImageBuilder != nil {
		return i.dockerfileImageBuilder.Build()
	}

	containerLockName := ContainerLockName(i.container.Name())
	if err := shluz.Lock(containerLockName, shluz.LockOptions{}); err != nil {
		return fmt.Errorf("failed to lock %s: %s", containerLockName, err)
	}
	defer shluz.Unlock(containerLockName)

	if debugDockerRunCommand() {
		runArgs, err := i.container.prepareRunArgs()
		if err != nil {
			return err
		}

		fmt.Printf("Docker run command:\ndocker run %s\n", strings.Join(runArgs, " "))

		if len(i.container.prepareAllRunCommands()) != 0 {
			fmt.Printf("Decoded command:\n%s\n", strings.Join(i.container.prepareAllRunCommands(), " && "))
		}
	}

	if containerRunErr := i.container.run(); containerRunErr != nil {
		if strings.HasPrefix(containerRunErr.Error(), "container run failed") {
			if options.IntrospectBeforeError {
				logboek.Default.LogFDetails("Launched command: %s\n", strings.Join(i.container.prepareAllRunCommands(), " && "))

				if err := logboek.WithRawStreamsOutputModeOn(i.introspectBefore); err != nil {
					return fmt.Errorf("introspect error failed: %s", err)
				}
			} else if options.IntrospectAfterError {
				if err := i.Commit(); err != nil {
					return fmt.Errorf("introspect error failed: %s", err)
				}

				logboek.Default.LogFDetails("Launched command: %s\n", strings.Join(i.container.prepareAllRunCommands(), " && "))

				if err := logboek.WithRawStreamsOutputModeOn(i.Introspect); err != nil {
					return fmt.Errorf("introspect error failed: %s", err)
				}
			}

			if err := i.container.rm(); err != nil {
				return fmt.Errorf("introspect error failed: %s", err)
			}
		}

		return containerRunErr
	}

	if err := i.Commit(); err != nil {
		return err
	}

	if err := i.container.rm(); err != nil {
		return err
	}

	if builtId, err := i.GetBuiltId(); err != nil {
		return fmt.Errorf("unable to get built id: %s", err)
	} else if inspect, err := i.LocalDockerServerRuntime.GetImageInspect(builtId); err != nil {
		return err
	} else {
		i.SetInspect(inspect)
		i.SetStagesStorageImageInfo(image.NewInfoFromInspect(i.Name(), inspect))
	}

	return nil
}

func (i *StageImage) Commit() error {
	builtId, err := i.container.commit()
	if err != nil {
		return err
	}

	i.buildImage = newBuildImage(builtId, i.LocalDockerServerRuntime)

	return nil
}

func (i *StageImage) Introspect() error {
	if err := i.container.introspect(); err != nil {
		return err
	}

	return nil
}

func (i *StageImage) introspectBefore() error {
	if err := i.container.introspectBefore(); err != nil {
		return err
	}

	return nil
}

func (i *StageImage) MustResetInspect() error {
	if i.buildImage != nil {
		return i.buildImage.MustResetInspect()
	} else {
		return i.baseImage.MustResetInspect()
	}
}

func (i *StageImage) GetInspect() *types.ImageInspect {
	if i.buildImage != nil {
		return i.buildImage.GetInspect()
	} else {
		return i.baseImage.GetInspect()
	}

}

func (i *StageImage) MustGetBuiltId() string {
	builtId, err := i.GetBuiltId()
	if err != nil {
		panic(fmt.Sprintf("error getting built id for %s: %s", i.Name(), err))
	}
	return builtId
}

func (i *StageImage) GetBuiltId() (string, error) {
	if i.dockerfileImageBuilder != nil {
		return i.dockerfileImageBuilder.GetBuiltId()
	} else {
		return i.buildImage.Name(), nil
	}
}

func (i *StageImage) TagBuiltImage(name string) error {
	buildImageId, err := i.GetBuiltId()
	if err != nil {
		return err
	}
	return docker.CliTag(buildImageId, i.name)
}

func (i *StageImage) Tag(name string) error {
	return docker.CliTag(i.GetID(), name)
}

func (i *StageImage) Pull() error {
	if err := docker.CliPullWithRetries(i.name); err != nil {
		return err
	}

	i.baseImage.UnsetInspect()

	return nil
}

func (i *StageImage) Push() error {
	return docker.CliPushWithRetries(i.name)
}

func (i *StageImage) Import(name string) error {
	importedImage := newBaseImage(name, i.LocalDockerServerRuntime)

	if err := docker.CliPullWithRetries(name); err != nil {
		return err
	}

	importedImageId := importedImage.GetStagesStorageImageInfo().ID

	if err := docker.CliTag(importedImageId, i.name); err != nil {
		return err
	}

	if err := docker.CliRmi(name); err != nil {
		return err
	}

	return nil
}

func (i *StageImage) Export(name string) error {
	if err := logboek.Info.LogProcess(fmt.Sprintf("Tagging %s", name), logboek.LevelLogProcessOptions{}, func() error {
		return i.Tag(name)
	}); err != nil {
		return err
	}

	defer func() {
		if err := logboek.Info.LogProcess(fmt.Sprintf("Untagging %s", name), logboek.LevelLogProcessOptions{}, func() error {
			return docker.CliRmi(name)
		}); err != nil {
			// TODO: errored image state
			logboek.Error.LogF("Unable to remote temporary image %q: %s", name, err)
		}
	}()

	if err := logboek.Info.LogProcess(fmt.Sprintf("Pushing %s", name), logboek.LevelLogProcessOptions{}, func() error {
		return docker.CliPushWithRetries(name)
	}); err != nil {
		return err
	}

	return nil
}

func (i *StageImage) DockerfileImageBuilder() *DockerfileImageBuilder {
	if i.dockerfileImageBuilder == nil {
		i.dockerfileImageBuilder = NewDockerfileImageBuilder()
	}
	return i.dockerfileImageBuilder
}

func debugDockerRunCommand() bool {
	return os.Getenv("WERF_DEBUG_DOCKER_RUN_COMMAND") == "1"
}
