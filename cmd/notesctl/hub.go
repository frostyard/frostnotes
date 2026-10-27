package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

func newHubCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Serve vaults, manage devices and join a hub",
	}
	cmd.AddCommand(newHubStatusCmd(opts), newHubServeCmd(opts), newHubVaultsCmd(opts),
		newHubPendingCmd(opts), newHubApproveCmd(opts), newHubDevicesCmd(opts),
		newHubRevokeCmd(opts), newHubJoinCmd(opts), newHubURLCmd(opts), newHubRelocateCmd(opts))
	return cmd
}

func newHubStatusCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the hub's status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			status, err := c.Hub().Status(cmd.Context(), &api.HubStatusParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(status); err != nil || written {
				return err
			}
			hub := "hub: none yet (serve a vault first)"
			if status.Hub != "" {
				hub = "hub " + status.Hub
			}
			listen := "not listening: start notesd with --hub ADDR"
			if status.Listen != nil {
				listen = "listening on " + *status.Listen
			}
			if status.URL != nil {
				listen += "\ndevices use " + *status.URL
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n%d vaults, %d devices, %d connected\n",
				hub, listen, status.Vaults, status.Devices, status.Connected)
			return err
		},
	}
}

func newHubURLCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "url URL",
		Short: "Record the URL devices should use for this hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			status, err := c.Hub().SetURL(cmd.Context(), &api.HubSetURLParams{URL: args[0]})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(status); err != nil || written {
				return err
			}
			url := "the URL they paired with"
			if status.URL != nil {
				url = *status.URL
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "devices use %s\n", url)
			return err
		},
	}
}

func newHubRelocateCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "relocate URL NEW-URL",
		Short: "Verify and update a paired hub's URL",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			hub, err := c.Sync().Relocate(cmd.Context(), &api.SyncRelocateParams{Hub: args[0], URL: args[1]})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(hub); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "hub %s is at %s now\n", hub.Hub, hub.URL)
			return err
		},
	}
}

func newHubServeCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve a registered vault on the hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				served, err := c.Hub().Serve(cmd.Context(), &api.HubServeParams{VaultID: vault})
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(served); err != nil || written {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "serving %s as %s\n", served.Name, served.Sync)
				return err
			})
		},
	}
}

func newHubVaultsCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "vaults",
		Short: "List vaults served by the hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			vaults, err := c.Hub().Vaults(cmd.Context(), &api.HubVaultsParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(vaults); err != nil || written {
				return err
			}
			if len(vaults) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no vaults served")
				return err
			}
			for _, vault := range vaults {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\n", vault.Name, vault.Sync, vault.Seq); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newHubPendingCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "pending",
		Short: "List devices waiting to pair",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			pending, err := c.Hub().Pending(cmd.Context(), &api.HubPendingParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(pending); err != nil || written {
				return err
			}
			if len(pending) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no pending pairings")
				return err
			}
			for _, pairing := range pending {
				approved := ""
				if pairing.Approved {
					approved = "\tapproved"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s%s\n", pairing.Code,
					pairing.Name, pairing.Addr, pairing.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"), approved); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newHubApproveCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "approve CODE",
		Short: "Approve a pending device pairing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			pairing, err := c.Hub().Approve(cmd.Context(), &api.HubApproveParams{Code: args[0]})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "approved %s (%s)\n", pairing.Name, pairing.Code)
			return err
		},
	}
}

func newHubDevicesCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "devices",
		Short: "List devices paired with the hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			devices, err := c.Hub().Devices(cmd.Context(), &api.HubDevicesParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(devices); err != nil || written {
				return err
			}
			if len(devices) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no paired devices")
				return err
			}
			for _, device := range devices {
				seen := "never"
				if device.SeenAt != nil {
					seen = device.SeenAt.UTC().Format("2006-01-02T15:04:05Z")
				}
				state := "offline"
				if device.Revoked {
					state = "revoked"
				} else if device.Connected {
					state = "connected"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\n", device.ID,
					device.Name, device.PairedAt.UTC().Format("2006-01-02T15:04:05Z"), seen, state); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newHubRevokeCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke ID",
		Short: "Revoke a paired device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("device ID %q is not a number", args[0])
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Hub().Revoke(cmd.Context(), &api.HubRevokeParams{Device: id}); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "revoked device %d\n", id)
			return err
		},
	}
}

func newHubJoinCmd(opts *rootOptions) *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "join URL NAME",
		Short: "Pair with a hub and sync a vault",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				url := strings.TrimSuffix(args[0], "/")
				params := &api.SyncPairParams{URL: url}
				if cmd.Flags().Changed("device") {
					params.Name = &device
				}
				out := cmd.OutOrStdout()
				paired, err := hasHub(cmd.Context(), c.Sync(), url)
				if err != nil {
					return err
				}
				if !paired {
					if err := pairNew(cmd.Context(), out, c.Sync(), params); err != nil {
						return err
					}
				}
				join := &api.SyncJoinParams{VaultID: vault, URL: url, Name: args[1]}
				status, err := c.Sync().Join(cmd.Context(), join)
				if paired && errors.Is(err, api.ErrRevoked) {
					// The hub refused the token the device kept, or the device
					// lost it: pair again, once.
					if _, err := fmt.Fprintf(out, "the hub at %s refused this device's token; pairing again\n", url); err != nil {
						return err
					}
					if err := pairNew(cmd.Context(), out, c.Sync(), params); err != nil {
						return err
					}
					status, err = c.Sync().Join(cmd.Context(), join)
				}
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "syncing %s at %s: %s\n", status.HubVault, status.URL, status.State)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&device, "device", "", "This device's name on the hub (default: host name)")
	return cmd
}

// hasHub reports whether this device paired with the hub at url. A
// pairing it holds may still be dead: the hub may have revoked the token.
func hasHub(ctx context.Context, client api.SyncClient, url string) (bool, error) {
	hubs, err := client.Hubs(ctx, &api.SyncHubsParams{})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(hubs, func(h api.SyncHub) bool { return h.URL == url }), nil
}

// pairNew pairs with a hub and waits for the approval of the pairing.
// A pairing with a hub paired already replaces its token.
func pairNew(ctx context.Context, out io.Writer, client api.SyncClient, params *api.SyncPairParams) error {
	pairing, err := client.Pair(ctx, params)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "approve this device on the hub: notesctl hub approve %s\n", pairing.Code); err != nil {
		return err
	}
	for {
		result, err := client.Paired(ctx, &api.SyncPairedParams{Pairing: pairing.Pairing})
		if err != nil {
			return err
		}
		switch result.State {
		case api.PairingStateApproved:
			_, err := fmt.Fprintf(out, "paired with hub %s\n", result.Hub)
			return err
		case api.PairingStateExpired:
			return fmt.Errorf("pairing expired")
		case api.PairingStatePending:
			// notesd long-polls for each call; ask again without sleeping.
		default:
			return fmt.Errorf("unknown pairing state %q", result.State)
		}
	}
}
