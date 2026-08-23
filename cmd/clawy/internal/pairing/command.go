// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package pairing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/pkg/pairing"
)

// NewPairingCommand builds the `clawy pairing` command group implementing the
// OpenClaw-style DM pairing approval flow.
func NewPairingCommand() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:     "pairing",
		Aliases: []string{"pair"},
		Short:   "Manage DM pairing requests",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			// pairing.NewManager is rooted at the workspace and stores its
			// state at <workspace>/pairing/pairing.json — the same location
			// the gateway runtime uses.
			workspace = cfg.WorkspacePath()
			return nil
		},
	}

	cmd.AddCommand(
		newListCommand(func() string { return workspace }),
		newApproveCommand(func() string { return workspace }),
		newRejectCommand(func() string { return workspace }),
	)

	return cmd
}

func newListCommand(workspace func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List pending pairing requests",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			mgr := pairing.NewManager(workspace())
			reqs, err := mgr.List()
			if err != nil {
				return err
			}
			if len(reqs) == 0 {
				fmt.Println("No pending pairing requests.")
				return nil
			}
			sort.Slice(reqs, func(i, j int) bool { return reqs[i].CreatedAt.Before(reqs[j].CreatedAt) })
			fmt.Printf("Pending Pairing Requests (%d):\n", len(reqs))
			fmt.Println("-----------------------------")
			for _, req := range reqs {
				fmt.Printf("  %s | %s | code=%s | requested=%s\n",
					req.Channel, req.SenderID, req.Code, req.CreatedAt.Format("2006-01-02 15:04"))
				if name := strings.TrimSpace(req.DisplayName); name != "" {
					fmt.Printf("      display name: %s\n", name)
				}
			}
			fmt.Println("\nApprove with: clawy pairing approve <channel> <code>")
			return nil
		},
	}
}

func newApproveCommand(workspace func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <channel> <code>",
		Short: "Approve a pending pairing request",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			mgr := pairing.NewManager(workspace())
			req, err := mgr.Approve(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Printf("✓ Paired %s on %s\n", req.SenderID, args[0])
			fmt.Printf("  Add \"%s:%s\" to the channel's allow_from list to persist access.\n",
				args[0], req.SenderID)
			return nil
		},
	}
}

func newRejectCommand(workspace func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <channel> <code>",
		Short: "Reject a pending pairing request",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			mgr := pairing.NewManager(workspace())
			if _, err := mgr.Reject(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("✓ Rejected pairing request %s/%s\n", args[0], args[1])
			return nil
		},
	}
}