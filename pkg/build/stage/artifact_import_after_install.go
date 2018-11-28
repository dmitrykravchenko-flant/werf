package stage

import "github.com/flant/dapp/pkg/config"

func GenerateArtifactImportAfterInstallStage(dimgBaseConfig *config.DimgBase) Interface {
	imports := getImports(dimgBaseConfig, &getImportsOptions{After: "install"})
	if len(imports) != 0 {
		return newArtifactImportAfterInstallStage(imports)
	}

	return nil
}

func newArtifactImportAfterInstallStage(imports []*config.ArtifactImport) *ArtifactImportAfterInstallStage {
	s := &ArtifactImportAfterInstallStage{}
	s.ArtifactImportBaseStage = newArtifactImportBaseStage(imports)
	return s
}

type ArtifactImportAfterInstallStage struct {
	*ArtifactImportBaseStage
}

func (s *ArtifactImportAfterInstallStage) Name() string {
	return "after_install_artifact"
}
