// +build off

package python

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"sourcegraph.com/sourcegraph/srclib/config"
	"sourcegraph.com/sourcegraph/srclib/container"
	"sourcegraph.com/sourcegraph/srclib/dep2"
	"sourcegraph.com/sourcegraph/srclib/unit"
)

func init() {
	dep2.RegisterLister(&DistPackage{}, dep2.DockerLister{defaultPythonEnv})
	dep2.RegisterResolver(pythonRequirementTargetType, defaultPythonEnv)
}

func (p *pythonEnv) BuildLister(dir string, u unit.SourceUnit, c *config.Repository) (*container.Command, error) {
	var dockerfile []byte
	var cmd []string
	var err error

	hardcodedDeps, hardcoded := hardcodedDep[repoUnit{Repo: c.URI, Unit: u.Name(), UnitType: unit.Type(u)}]
	if hardcoded {
		dockerfile = []byte(`FROM ubuntu:14.04`)
		cmd = []string{"echo", ""}
	} else {
		dockerfile, err = p.pydepDockerfile()
		if err != nil {
			return nil, err
		}
		cmd = []string{"pydep-run.py", "dep", filepath.Join(srcRoot, u.RootDir())}
	}

	return &container.Command{
		Container: container.Container{
			Dockerfile: dockerfile,
			RunOptions: []string{"-v", dir + ":" + srcRoot},
			Cmd:        cmd,
		},
		Transform: func(orig []byte) ([]byte, error) {
			if hardcoded {
				return json.Marshal(hardcodedDeps)
			}

			var reqs []requirement
			err := json.NewDecoder(bytes.NewReader(orig)).Decode(&reqs)
			if err != nil {
				return nil, err
			}
			reqs, ignoredReqs := pruneReqs(reqs)
			if len(ignoredReqs) > 0 {
				ignoredKeys := make([]string, len(ignoredReqs))
				for r, req := range ignoredReqs {
					ignoredKeys[r] = req.Key
				}
				log.Printf("(warn) ignoring dependencies %v because repo URL absent", ignoredKeys)
			}

			deps := make([]*dep2.RawDependency, 0)
			for _, req := range reqs {
				deps = append(deps, &dep2.RawDependency{
					TargetType: pythonRequirementTargetType,
					Target:     req,
				})
			}
			return json.Marshal(deps)
		},
	}, nil
}

func (p *pythonEnv) Resolve(dep *dep2.RawDependency, c *config.Repository) (*dep2.ResolvedTarget, error) {
	switch dep.TargetType {
	case pythonRequirementTargetType:
		var req requirement
		reqJson, _ := json.Marshal(dep.Target)
		json.Unmarshal(reqJson, &req)

		toUnit := req.DistPackage()
		return &dep2.ResolvedTarget{
			ToRepoCloneURL: req.RepoURL,
			ToUnit:         toUnit.Name(),
			ToUnitType:     unit.Type(toUnit),
		}, nil
	default:
		return nil, fmt.Errorf("Unexpected target type for Python: %+v", dep.TargetType)
	}
}

func pruneReqs(reqs []requirement) (kept, ignored []requirement) {
	for _, req := range reqs {
		if req.RepoURL != "" { // cannot resolve dependencies with no clone URL
			kept = append(kept, req)
		} else {
			ignored = append(ignored, req)
		}
	}
	return
}

const pythonRequirementTargetType = "python-requirement"
