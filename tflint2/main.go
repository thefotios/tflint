package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/configs"
	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/lang"
	"github.com/hashicorp/terraform/terraform"
	"github.com/k0kubun/pp"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// TODO: Support TF_ variables
// @see https://www.terraform.io/docs/configuration/environment-variables.html

func main() {
	loader, err := configload.NewLoader(&configload.Config{
		ModulesDir: ".terraform/modules",
	})
	if err != nil {
		panic(err)
	}

	cfg := loadConfig(loader)
	pp.Print(cfg.Children["test"].Module)
	vals := loadTFVars(loader, "terraform.tfvars")
	variableValues := prepareVariables(cfg.Module.Variables, vals)

	ctx := terraform.BuiltinEvalContext{
		PathValue: addrs.RootModuleInstance,
		Evaluator: &terraform.Evaluator{
			Meta: &terraform.ContextMeta{
				Env: getWorkspace(),
			},
			Config:             cfg,
			VariableValues:     variableValues,
			VariableValuesLock: &sync.Mutex{},
		},
	}

	body, _, diags := cfg.Module.ManagedResources["aws_instance.web"].Config.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{
				Name: "instance_type",
			},
		},
	})
	if diags.HasErrors() {
		panic(diags)
	}

	expr := body.Attributes["instance_type"].Expr
	checkExpr(expr)

	val, hcldiags := ctx.EvaluateExpr(expr, cty.DynamicPseudoType, nil)
	if hcldiags.HasErrors() {
		panic(hcldiags.Err())
	}

	var ret string
	err = gocty.FromCtyValue(val, &ret)
	if err != nil {
		panic(err)
	}

	pp.Print(ret)
}

// configload LoadConfig()
func loadConfig(loader *configload.Loader) *configs.Config {
	rootMod, diags := loader.Parser().LoadConfigDir(".")
	if diags.HasErrors() {
		panic(diags)
	}
	cfg, diags := configs.BuildConfig(rootMod, configs.ModuleWalkerFunc(
		func(req *configs.ModuleRequest) (*configs.Module, *version.Version, hcl.Diagnostics) {
			sum := md5.Sum([]byte("1." + req.Name + ";" + req.SourceAddr))
			dir := ".terraform/modules/" + hex.EncodeToString(sum[:])
			mod, diags := loader.Parser().LoadConfigDir(dir)

			return mod, nil, diags
		},
	))
	if diags.HasErrors() {
		panic(diags)
	}
	return cfg
}

func loadTFVars(loader *configload.Loader, file string) terraform.InputValues {
	vals, diags := loader.Parser().LoadValuesFile(file)
	if diags.HasErrors() {
		panic(diags)
	}

	ret := make(terraform.InputValues)
	for k, v := range vals {
		ret[k] = &terraform.InputValue{
			Value:      v,
			SourceType: terraform.ValueFromFile,
		}
	}
	return ret
}

func prepareVariables(configVars map[string]*configs.Variable, tfvars terraform.InputValues) map[string]map[string]cty.Value {
	// terraform/context NewContext()
	variables := terraform.DefaultVariableValues(configVars).Override(tfvars)

	// terraform/graph_context_walker init()
	variableValues := make(map[string]map[string]cty.Value)
	variableValues[""] = make(map[string]cty.Value)
	for k, iv := range variables {
		variableValues[""][k] = iv.Value
	}
	return variableValues
}

// terraform/command (*Meta) Workspace()
func getWorkspace() string {
	if envVar := os.Getenv("TF_WORKSPACE"); envVar != "" {
		return envVar
	}

	envData, _ := ioutil.ReadFile(".terraform/environment")
	current := string(bytes.TrimSpace(envData))
	if current == "" {
		current = "default"
	}

	return current
}

func checkExpr(expr hcl.Expression) {
	refs, diags := lang.ReferencesInExpr(expr)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	for _, ref := range refs {
		switch v := ref.Subject.(type) {
		case addrs.InputVariable:
			// noop
		case addrs.TerraformAttr:
			// noop
		default:
			panic(fmt.Errorf("Unsupported reference: %s", v))
		}
	}
}
