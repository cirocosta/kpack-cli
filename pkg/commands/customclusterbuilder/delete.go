package customclusterbuilder

import (
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pivotal/build-service-cli/pkg/commands"
)

func NewDeleteCommand(cmdContext commands.ContextProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <name>",
		Short:   "Delete a custom cluster builder",
		Long:    "Delete a custom cluster builder from the cluster.",
		Example: "tbctl ccb delete my-builder",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdContext.Initialize(); err != nil {
				return err
			}

			err := cmdContext.KpackClient().ExperimentalV1alpha1().CustomClusterBuilders().Delete(args[0], &metav1.DeleteOptions{})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "\"%s\" deleted\n", args[0])
			return err
		},
		SilenceUsage: true,
	}

	return cmd
}
