package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/acarl005/stripansi"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/pomdtr/sunbeam/internal/extensions"
	"github.com/pomdtr/sunbeam/internal/schemas"
	"github.com/pomdtr/sunbeam/internal/utils"
	"github.com/pomdtr/sunbeam/pkg/sunbeam"
)

type Runner struct {
	embed         Page
	form          *Form
	width, height int
	cancel        context.CancelFunc

	extension extensions.Extension
	command   sunbeam.CommandSpec
	input     sunbeam.Payload
}

func NewRunner(extension extensions.Extension, input sunbeam.Payload) *Runner {
	var embed Page
	command, ok := extension.Command(input.Command)
	if ok {
		switch command.Mode {
		case sunbeam.CommandModeSearch, sunbeam.CommandModeFilter:
			list := NewList()
			list.SetEmptyText("Loading...")
			if input.Query != "" {
				list.SetQuery(input.Query)
			}

			embed = list
		case sunbeam.CommandModeDetail:
			embed = NewDetail("")
		default:
			embed = NewErrorPage(fmt.Errorf("invalid view type"))
		}
	} else {
		embed = NewErrorPage(fmt.Errorf("command %s not found", input.Command))
	}

	return &Runner{
		embed:     embed,
		extension: extension,
		command:   command,
		input:     input,
	}
}

func (c *Runner) SetIsLoading(isLoading bool) tea.Cmd {
	switch page := c.embed.(type) {
	case *Detail:
		return page.SetIsLoading(isLoading)
	case *List:
		return page.SetIsLoading(isLoading)
	}

	return nil
}

func (c *Runner) Init() tea.Cmd {
	termenv.DefaultOutput().SetWindowTitle(fmt.Sprintf("%s - %s", c.command.Title, c.extension.Manifest.Title))
	return tea.Batch(c.Reload(), c.embed.Init())
}

func (c *Runner) Focus() tea.Cmd {
	if c.embed == nil {
		return nil
	}
	termenv.DefaultOutput().SetWindowTitle(fmt.Sprintf("%s - %s", c.command.Title, c.extension.Manifest.Title))
	return c.embed.Focus()
}

func (c *Runner) Blur() tea.Cmd {
	c.cancel()
	return nil
}

func (c *Runner) SetSize(w int, h int) {
	c.width = w
	c.height = h

	if c.form != nil {
		c.form.SetSize(w, h)
	}

	c.embed.SetSize(w, h)
}

func (c *Runner) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if c.form != nil {
				c.form = nil
				return c, c.embed.Focus()
			}

			if c.embed != nil {
				break
			}
			return c, PopPageCmd
		case "ctrl+s":
			editCmd := exec.Command("sunbeam", "edit", c.extension.Entrypoint)
			return c, tea.ExecProcess(editCmd, func(err error) tea.Msg {
				if err != nil {
					return err
				}

				extension, err := extensions.LoadExtension(c.extension.Entrypoint)
				if err != nil {
					return err
				}
				c.extension = extension

				return ReloadMsg{}
			})
		case "ctrl+r":
			return c, func() tea.Msg {
				manifest, err := extensions.ExtractManifest(c.extension.Entrypoint)
				if err != nil {
					return err
				}
				c.extension.Manifest = manifest

				return ReloadMsg{}
			}
		}
	case ReloadMsg:
		return c, c.Reload()
	case Page:
		c.embed = msg
		c.embed.SetSize(c.width, c.height)
		return c, c.embed.Init()
	case sunbeam.Action:
		switch msg.Type {
		case sunbeam.ActionTypeRun:
			command, ok := c.extension.Command(msg.Run.Command)
			if !ok {
				c.embed = NewErrorPage(fmt.Errorf("command %s not found", msg.Run.Command))
				c.embed.SetSize(c.width, c.height)
				return c, c.embed.Init()
			}

			missing := FindMissingInputs(command.Params, msg.Run.Params)
			for _, param := range missing {
				if param.Optional {
					continue
				}

				c.form = NewForm(func(values map[string]any) tea.Msg {
					params := make(map[string]any)
					for k, v := range msg.Run.Params {
						params[k] = v
					}

					for k, v := range values {
						params[k] = v
					}

					props := msg.Run
					props.Params = params

					return sunbeam.Action{
						Title:  msg.Title,
						Type:   sunbeam.ActionTypeRun,
						Run:    props,
						Reload: msg.Reload,
					}
				}, missing...)

				c.form.SetSize(c.width, c.height)
				return c, tea.Sequence(c.form.Init(), c.form.Focus())
			}
			c.form = nil

			input := sunbeam.Payload{
				Command:     msg.Run.Command,
				Preferences: c.input.Preferences,
				Params:      make(map[string]any),
			}

			for k, v := range msg.Run.Params {
				input.Params[k] = v
			}

			switch command.Mode {
			case sunbeam.CommandModeSearch, sunbeam.CommandModeFilter, sunbeam.CommandModeDetail:
				runner := NewRunner(c.extension, input)

				return c, PushPageCmd(runner)
			case sunbeam.CommandModeSilent:
				return c, func() tea.Msg {
					_, err := c.extension.Output(input)

					if err != nil {
						return PushPageMsg{NewErrorPage(err)}
					}

					if msg.Run.Reload {
						return ReloadMsg{}
					}

					if msg.Run.Exit {
						return ExitMsg{}
					}

					return nil
				}
			case sunbeam.CommandModeTTY:
				cmd, err := c.extension.Cmd(input)
				if err != nil {
					c.embed = NewErrorPage(err)
					c.embed.SetSize(c.width, c.height)
					return c, c.embed.Init()
				}

				return c, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						return PushPageMsg{NewErrorPage(err)}
					}

					if msg.Run.Reload {
						c.embed.Focus()
						return ReloadMsg{}
					}

					if msg.Run.Exit {
						return ExitMsg{}
					}

					termenv.DefaultOutput().SetWindowTitle(fmt.Sprintf("%s - %s", c.command.Title, c.extension.Manifest.Title))
					return c.embed.Focus()
				})
			}
		case sunbeam.ActionTypeEdit:
			editCmd := exec.Command("sunbeam", "edit", msg.Edit.Path)
			return c, tea.ExecProcess(editCmd, func(err error) tea.Msg {
				if err != nil {
					return err
				}

				if msg.Edit.Reload {
					c.embed.Focus()
					return c.Reload()
				}

				if msg.Edit.Exit {
					return ExitMsg{}
				}

				return c.embed.Focus()
			})
		case sunbeam.ActionTypeCopy:
			return c, func() tea.Msg {
				if err := clipboard.WriteAll(msg.Copy.Text); err != nil {
					return err
				}

				if msg.Copy.Exit {
					return ExitMsg{}
				}

				return ShowNotificationMsg{"Copied!"}
			}
		case sunbeam.ActionTypeOpen:
			return c, func() tea.Msg {
				if msg.Open.Url != "" {
					if err := utils.Open(msg.Open.Url); err != nil {
						return err
					}

					return ExitMsg{}
				} else if msg.Open.Path != "" {
					if err := utils.Open(fmt.Sprintf("file://%s", msg.Open.Path)); err != nil {
						return err
					}

					return ExitMsg{}
				} else {
					return fmt.Errorf("invalid target")
				}
			}
		case sunbeam.ActionTypeExit:
			return c, ExitCmd
		case sunbeam.ActionTypeReload:
			if c.input.Params == nil {
				c.input.Params = make(map[string]any)
			}

			for k, v := range msg.Reload.Params {
				c.input.Params[k] = v
			}

			return c, c.Reload()
		}

	case error:
		c.embed = NewErrorPage(msg)
		c.embed.SetSize(c.width, c.height)
		return c, c.embed.Init()
	}

	if c.form != nil {
		form, cmd := c.form.Update(msg)
		c.form = form.(*Form)
		return c, cmd
	}

	var cmd tea.Cmd
	c.embed, cmd = c.embed.Update(msg)
	return c, cmd
}

func (c *Runner) View() string {
	if c.form != nil {
		return c.form.View()
	}

	return c.embed.View()
}

func (c *Runner) Reload() tea.Cmd {
	return tea.Sequence(c.SetIsLoading(true), func() tea.Msg {
		if c.cancel != nil {
			c.cancel()
		}

		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		defer cancel()

		cmd, err := c.extension.CmdContext(ctx, c.input)
		if err != nil {
			return err
		}

		output, err := cmd.Output()
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return fmt.Errorf("command failed: %s", stripansi.Strip(string(exitErr.Stderr)))
			}

			return err
		}

		switch c.command.Mode {
		case sunbeam.CommandModeDetail:
			if err := schemas.ValidateDetail(output); err != nil {
				return err
			}

			var detail sunbeam.Detail
			if err := json.Unmarshal(output, &detail); err != nil {
				return err
			}

			if detail.Markdown != "" {
				page := NewDetail(detail.Markdown, detail.Actions...)
				page.Markdown = true
				return page
			}

			page := NewDetail(detail.Text, detail.Actions...)
			return page
		case sunbeam.CommandModeSearch, sunbeam.CommandModeFilter:
			if err := schemas.ValidateList(output); err != nil {
				return err
			}

			var list sunbeam.List
			if err := json.Unmarshal(output, &list); err != nil {
				return err
			}

			var page *List
			if embed, ok := c.embed.(*List); ok {
				page = embed
				page.SetItems(list.Items...)
				page.SetIsLoading(false)
				page.SetEmptyText(list.EmptyText)
				page.SetActions(list.Actions...)
				page.SetShowDetail(list.ShowDetail)
				page.SetAutoRefreshSeconds(list.AutoRefreshSeconds)

				if c.command.Mode == sunbeam.CommandModeSearch {
					page.OnQueryChange = func(query string) tea.Cmd {
						c.input.Query = query
						return c.Reload()
					}
					page.ResetSelection()
				}

				return nil
			}

			page = NewList(list.Items...)
			page.SetEmptyText(list.EmptyText)
			page.SetActions(list.Actions...)
			page.SetShowDetail(list.ShowDetail)
			if c.command.Mode == sunbeam.CommandModeSearch {
				page.OnQueryChange = func(query string) tea.Cmd {
					c.input.Query = query
					return c.Reload()
				}
			}

			return page
		default:
			return fmt.Errorf("invalid view type")
		}
	})
}
