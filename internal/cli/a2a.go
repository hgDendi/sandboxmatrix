package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/spf13/cobra"
)

// sharedGateway is a package-level A2A gateway used by both the a2a CLI
// commands and potentially other subsystems. For CLI debugging/testing the
// gateway lives only within the process lifetime.
var sharedGateway = a2a.New()

// newA2ACmd creates the "a2a" command group with its subcommands.
func newA2ACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "a2a",
		Short: "Agent-to-Agent messaging (debugging/testing)",
	}

	cmd.AddCommand(
		newA2ASendCmd(),
		newA2AReceiveCmd(),
		newA2ABroadcastCmd(),
	)
	return cmd
}

func newA2ASendCmd() *cobra.Command {
	var (
		from    string
		to      string
		msgType string
		payload string
	)

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message from one sandbox to another",
		RunE: func(_ *cobra.Command, _ []string) error {
			msg := &a2a.Message{
				From:    from,
				To:      to,
				Type:    msgType,
				Payload: payload,
			}
			if err := sharedGateway.Send(context.Background(), msg); err != nil {
				return err
			}
			fmt.Printf("Message sent from %q to %q (type: %s).\n", from, to, msgType)
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Sender sandbox name (required)")
	cmd.Flags().StringVar(&to, "to", "", "Recipient sandbox name (required)")
	cmd.Flags().StringVar(&msgType, "type", "", "Message type (required)")
	cmd.Flags().StringVar(&payload, "payload", "", "Message payload (JSON string)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newA2AReceiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "receive <sandbox>",
		Short: "Receive pending messages for a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			msgs, err := sharedGateway.Receive(context.Background(), args[0])
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				fmt.Println("No pending messages.")
				return nil
			}
			data, err := json.MarshalIndent(msgs, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal messages: %w", err)
			}
			fmt.Println(string(data))
			return nil
		},
	}
}

func newA2ABroadcastCmd() *cobra.Command {
	var (
		from    string
		targets string
		msgType string
		payload string
	)

	cmd := &cobra.Command{
		Use:   "broadcast",
		Short: "Broadcast a message to multiple sandboxes",
		RunE: func(_ *cobra.Command, _ []string) error {
			targetList := strings.Split(targets, ",")
			for i := range targetList {
				targetList[i] = strings.TrimSpace(targetList[i])
			}
			if err := sharedGateway.Broadcast(context.Background(), from, targetList, msgType, payload); err != nil {
				return err
			}
			fmt.Printf("Broadcast from %q to %d targets (type: %s).\n", from, len(targetList), msgType)
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Sender sandbox name (required)")
	cmd.Flags().StringVar(&targets, "targets", "", "Comma-separated list of target sandbox names (required)")
	cmd.Flags().StringVar(&msgType, "type", "", "Message type (required)")
	cmd.Flags().StringVar(&payload, "payload", "", "Message payload (JSON string)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("targets")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
