package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/peterwwillis/zop/internal/config"
)

// TemplateData is the data available to prompt templates.
type TemplateData struct {
	Input  string
	Config *config.Config
	Agent  config.AgentConfig
	Model  config.ModelConfig
	Env    map[string]string
}

func (c *Controller) resolveSystemPrompt() (string, error) {
	agent, err := c.cfg.GetAgent(c.agentName)
	if err != nil {
		return "", err
	}
	model, err := c.cfg.GetModel(agent.Model)
	if err != nil {
		return "", err
	}

	// Try Agent
	if agent.SystemPrompt != "" {
		return agent.SystemPrompt, nil
	}
	if agent.SystemPromptFile != "" {
		return c.readTemplateFile(agent.SystemPromptFile)
	}
	if agent.SystemPromptTemplate != "" {
		t, ok := c.cfg.Templates[agent.SystemPromptTemplate]
		if !ok {
			return "", fmt.Errorf("agent system_prompt_template %q not found", agent.SystemPromptTemplate)
		}
		if t.SystemPrompt != "" {
			return t.SystemPrompt, nil
		}
		if t.SystemPromptFile != "" {
			return c.readTemplateFile(t.SystemPromptFile)
		}
	}

	// Try Model
	if model.SystemPrompt != "" {
		return model.SystemPrompt, nil
	}
	if model.SystemPromptFile != "" {
		return c.readTemplateFile(model.SystemPromptFile)
	}
	if model.SystemPromptTemplate != "" {
		t, ok := c.cfg.Templates[model.SystemPromptTemplate]
		if !ok {
			return "", fmt.Errorf("model system_prompt_template %q not found", model.SystemPromptTemplate)
		}
		if t.SystemPrompt != "" {
			return t.SystemPrompt, nil
		}
		if t.SystemPromptFile != "" {
			return c.readTemplateFile(t.SystemPromptFile)
		}
	}

	return "", nil
}

func (c *Controller) resolveUserPromptTemplate() (string, error) {
	agent, err := c.cfg.GetAgent(c.agentName)
	if err != nil {
		return "", err
	}

	if agent.Prompt != "" {
		return agent.Prompt, nil
	}
	if agent.PromptFile != "" {
		return c.readTemplateFile(agent.PromptFile)
	}
	if agent.PromptTemplate != "" {
		t, ok := c.cfg.Templates[agent.PromptTemplate]
		if !ok {
			return "", fmt.Errorf("agent prompt_template %q not found", agent.PromptTemplate)
		}
		if t.Prompt != "" {
			return t.Prompt, nil
		}
		if t.PromptFile != "" {
			return c.readTemplateFile(t.PromptFile)
		}
	}

	return "", nil
}

func (c *Controller) readTemplateFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(c.configPath), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading template file %q: %w", path, err)
	}
	return string(data), nil
}

func (c *Controller) executeTemplate(tmplStr string, input string) (string, error) {
	if tmplStr == "" {
		tmplStr = input
		input = ""
	}

	agent, _ := c.cfg.GetAgent(c.agentName)
	model, _ := c.cfg.GetModel(agent.Model)

	data := TemplateData{
		Input:  input,
		Config: c.cfg,
		Agent:  agent,
		Model:  model,
		Env:    make(map[string]string),
	}

	for _, env := range os.Environ() {
		if i := strings.IndexByte(env, '='); i >= 0 {
			data.Env[env[:i]] = env[i+1:]
		}
	}

	tmpl, err := template.New("prompt").Funcs(template.FuncMap{
		"now":    time.Now,
		"date":   func() string { return time.Now().Format("2006-01-02") },
		"time":   func() string { return time.Now().Format("15:04:05") },
		"upper":  strings.ToUpper,
		"lower":  strings.ToLower,
		"trim":   strings.TrimSpace,
		"indent": func(n int, s string) string {
			indent := strings.Repeat(" ", n)
			return strings.ReplaceAll(s, "\n", "\n"+indent)
		},
	}).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}
