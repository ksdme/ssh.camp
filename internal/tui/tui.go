package tui

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ksdme/mail/internal/bus"
	"github.com/ksdme/mail/internal/config"
	"github.com/ksdme/mail/internal/models"
	"github.com/ksdme/mail/internal/tui/colors"
	"github.com/ksdme/mail/internal/tui/components/help"
	"github.com/ksdme/mail/internal/tui/email"
	"github.com/ksdme/mail/internal/tui/home"
	"github.com/uptrace/bun"
)

type mailboxRealTimeUpdate struct {
	mailbox int64
}

type mode int

const (
	Home mode = iota
	Email
)

// Represents the top most model.
type Model struct {
	db      *bun.DB
	account models.Account

	mode  mode
	home  home.Model
	email email.Model

	width  int
	height int

	KeyMap   KeyMap
	Colors   colors.ColorPalette
	Renderer *lipgloss.Renderer

	quitting bool
}

func NewModel(
	db *bun.DB,
	account models.Account,
	renderer *lipgloss.Renderer,
	colors colors.ColorPalette,
) Model {
	return Model{
		db:      db,
		account: account,

		mode:  Home,
		home:  home.NewModel(renderer, colors),
		email: email.NewModel(renderer, colors),

		KeyMap:   DefaultKeyMap(),
		Renderer: renderer,
		Colors:   colors,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.home.Init(),
		m.email.Init(),
		m.refreshMailboxes(false),
		m.listenToMailboxUpdate,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.home.Width = m.width - 12
		m.home.Height = m.height - 5

		m.email.Width = m.home.Width
		m.email.Height = m.home.Height

		m.home, _ = m.home.Update(msg)
		m.email, _ = m.email.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Quit):
			m.quitting = true
			return m, tea.Quit
		}

	case mailboxRealTimeUpdate:
		slog.Debug("received mailbox update", "mailbox", msg.mailbox)
		if msg.mailbox == m.home.SelectedMailbox.ID {
			return m, tea.Batch(
				m.refreshMails(m.home.SelectedMailbox),
				m.listenToMailboxUpdate,
				m.refreshMailboxes(true),
			)
		}
		return m, tea.Batch(
			m.listenToMailboxUpdate,
			m.refreshMailboxes(true),
		)

	case home.MailboxesRefreshedMsg:
		m.home, cmd = m.home.Update(msg)
		return m, cmd

	case home.MailboxSelectedMsg:
		return m, m.refreshMails(msg.Mailbox)

	case home.CreateRandomMailboxMsg:
		return m, m.createRandomMailbox

	case home.DeleteMailboxMsg:
		return m, m.deleteMailbox(msg.Mailbox)

	case email.MailSelectedMsg:
		m.mode = Email
		m.email, cmd = m.email.Update(msg)
		return m, tea.Batch(cmd, m.markMailSeen(msg.Mail))

	case email.MailDismissMsg:
		m.mode = Home
		return m, nil
	}

	if m.mode == Home {
		m.home, cmd = m.home.Update(msg)
		return m, cmd
	} else if m.mode == Email {
		m.email, cmd = m.email.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	// This lets us not leave behind lines at the end.
	if m.quitting {
		return ""
	}

	content := "loading"
	if m.mode == Home {
		content = m.home.View()
	} else if m.mode == Email {
		content = m.email.View()
	}

	bottom := help.View(m.Help(), m.Renderer, m.Colors)
	gap := m.width - lipgloss.Width(bottom) - lipgloss.Width(config.Settings.Signature) - 12
	if gap > 8 {
		bottom = lipgloss.JoinHorizontal(
			lipgloss.Left,
			bottom,
			m.Renderer.
				NewStyle().
				PaddingLeft(gap).
				Foreground(m.Colors.Muted).
				Render(config.Settings.Signature),
		)
	}

	return m.Renderer.
		NewStyle().
		Padding(2, 6).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Top,
				content,
				bottom,
			),
		)
}

func (m Model) Help() []key.Binding {
	var bindings []key.Binding

	if m.mode == Home {
		bindings = append(bindings, m.home.Help()...)
	} else if m.mode == Email {
		bindings = append(bindings, m.email.Help()...)
	}

	return append(bindings, m.KeyMap.Quit)
}

func (m Model) refreshMailboxes(passive bool) tea.Cmd {
	return func() tea.Msg {
		var mailboxes []home.MailboxWithUnread

		// TODO: The context should be bound to the ssh connection.
		var mailbox *models.Mailbox
		err := m.db.NewSelect().
			Model(mailbox).
			Column("mailbox.*").
			ColumnExpr("COUNT(mail.id) AS unread").
			Where("mailbox.account_id = ?", m.account.ID).
			Join("LEFT JOIN mails AS mail").
			JoinOn("mail.mailbox_id = mailbox.id").
			JoinOn("mail.seen = false").
			Order("mailbox.id DESC").
			Group("mailbox.id").
			Scan(context.Background(), &mailboxes)

		return home.MailboxesRefreshedMsg{
			Passive:   passive,
			Mailboxes: mailboxes,
			Err:       err,
		}
	}
}

func (m Model) refreshMails(mailbox *home.MailboxWithUnread) tea.Cmd {
	return func() tea.Msg {
		var mails []models.Mail

		// TODO: The context should be bound to the ssh connection.
		err := m.db.NewSelect().
			Model(&mails).
			Where("mailbox_id = ?", mailbox.ID).
			Order("id DESC").
			Scan(context.Background())

		return home.MailsRefreshedMsg{
			Mailbox: mailbox,
			Mails:   mails,
			Err:     err,
		}
	}
}

func (m Model) listenToMailboxUpdate() tea.Msg {
	slog.Debug("listening to mailbox updates", "account", m.account.ID)
	if value, aborted := bus.MailboxContentsUpdatedSignal.Wait(m.account.ID); !aborted {
		return mailboxRealTimeUpdate{value}
	}

	return nil
}

func (m Model) createRandomMailbox() tea.Msg {
	// TODO: The context should be bound to the ssh connection.
	_, err := models.CreateRandomMailbox(context.Background(), m.db, m.account)
	if err != nil {
		fmt.Println(err)
		// TODO: Handle this error.
		return nil
	}

	return m.refreshMailboxes(false)
}

func (m Model) deleteMailbox(mailbox *home.MailboxWithUnread) tea.Cmd {
	return func() tea.Msg {
		// TODO: The context should be bound to the ssh connection.
		_, err := m.db.
			NewDelete().
			Model(&models.Mailbox{}).
			Where("id = ?", mailbox.ID).
			Exec(context.Background())
		if err != nil {
			return nil
		}

		_, err = m.db.
			NewDelete().
			Model(&models.Mail{}).
			Where("mailbox_id = ?", mailbox.ID).
			Exec(context.Background())
		if err != nil {
			return nil
		}

		return m.refreshMailboxes(false)()
	}
}

func (m Model) markMailSeen(mail models.Mail) tea.Cmd {
	return func() tea.Msg {
		if !mail.Seen {
			mail.Seen = true

			// TODO: The context should be bound to the ssh connection.
			_, err := m.db.NewUpdate().Model(&mail).WherePK().Exec(context.Background())
			if err != nil {
				fmt.Println(err)
			}

			if m.home.SelectedMailbox != nil && mail.MailboxID == m.home.SelectedMailbox.ID {
				m.home.SelectedMailbox.Unread -= 1
			}
		}

		return nil
	}
}

type KeyMap struct {
	Quit key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "q"),
			key.WithHelp("q", "quit"),
		),
	}
}
