package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	beadCreateTicket          string
	beadCreateRig             string
	beadCreateType            string
	beadCreatePriority        string
	beadCreateDescription     string
	beadCreateDescriptionFile string
	beadCreateLabels          []string
	beadCreateDryRun          bool
)

var beadCreateCmd = &cobra.Command{
	Use:   "create <title>",
	Short: "Create a bead with a readable ID",
	Long: `Create a bead whose ID names the work, not a random hash.

The ID is <prefix>-<ticket>-<slug>, for example cf-eng-3243-fix-propelauth-refresh.
The ticket is the Linear ID from --ticket, or the first one found in the title.
Without a ticket the ID is <prefix>-<slug>. A taken ID gets a -2, -3 suffix.

The prefix comes from the target beads database: the rig named by --rig, the rig
you are standing in, or the town (hq) when neither applies.

Prints only the new ID, so scripts can capture it. Use --dry-run to see the ID
without creating anything.

Examples:
  gt bead create --ticket ENG-3243 "Fix PropelAuth refresh on 4xx" --rig cfx
  gt bead create "ENG-3243: customer dashboard token refresh error" --rig cfx
  gt bead create "Reply to Ben about settlement tables"
  B=$(gt bead create --ticket ENG-3243 --rig cfx "Fix PropelAuth refresh" -d "details")`,
	Args: cobra.ExactArgs(1),
	RunE: runBeadCreate,
}

func init() {
	beadCreateCmd.Flags().StringVar(&beadCreateTicket, "ticket", "", "Linear ticket ID to lead the bead ID (e.g. ENG-3243)")
	beadCreateCmd.Flags().StringVar(&beadCreateRig, "rig", "", "Rig whose beads database receives the bead (default: the rig you are in, else the town)")
	beadCreateCmd.Flags().StringVar(&beadCreateType, "type", "task", "Bead type (task, bug, feature, epic, chore)")
	beadCreateCmd.Flags().StringVarP(&beadCreatePriority, "priority", "p", "2", "Priority 0-4")
	beadCreateCmd.Flags().StringVarP(&beadCreateDescription, "description", "d", "", "Description text")
	beadCreateCmd.Flags().StringVar(&beadCreateDescriptionFile, "description-file", "", "Read the description from a file")
	beadCreateCmd.Flags().StringArrayVar(&beadCreateLabels, "label", nil, "Label to add (repeatable)")
	beadCreateCmd.Flags().BoolVar(&beadCreateDryRun, "dry-run", false, "Print the ID that would be used and create nothing")
	beadCmd.AddCommand(beadCreateCmd)
}

func runBeadCreate(cmd *cobra.Command, args []string) error {
	title := strings.TrimSpace(args[0])
	if title == "" {
		return fmt.Errorf("title must not be empty")
	}
	if beadCreateDescription != "" && beadCreateDescriptionFile != "" {
		return fmt.Errorf("use --description or --description-file, not both")
	}
	description := beadCreateDescription
	if beadCreateDescriptionFile != "" {
		raw, err := os.ReadFile(beadCreateDescriptionFile)
		if err != nil {
			return fmt.Errorf("reading description file: %w", err)
		}
		description = string(raw)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding the Gas Town workspace: %w", err)
	}
	targetDir, err := beadCreateTargetDir(townRoot, beadCreateRig)
	if err != nil {
		return err
	}

	prefixOut, err := BdCmd("config", "get", "issue_prefix").Dir(targetDir).Output()
	if err != nil {
		return fmt.Errorf("reading the bead ID prefix from %s: %w", targetDir, err)
	}
	prefix := strings.TrimSpace(string(prefixOut))

	id, err := beads.ReadableID(prefix, beadCreateTicket, title, func(candidate string) (bool, error) {
		return beadExists(targetDir, candidate)
	})
	if err != nil {
		return err
	}

	if beadCreateDryRun {
		fmt.Println(id)
		return nil
	}

	createArgs := []string{
		"create",
		"--id=" + id,
		"--title=" + title,
		"--type=" + beadCreateType,
		"--priority=" + beadCreatePriority,
		"--silent",
	}
	if beads.NeedsForceForID(id) {
		createArgs = append(createArgs, "--force")
	}
	if description != "" {
		createArgs = append(createArgs, "--description="+description)
	}
	for _, label := range beadCreateLabels {
		createArgs = append(createArgs, "--label="+label)
	}
	if err := BdCmd(createArgs...).Dir(targetDir).WithAutoCommit().Run(); err != nil {
		return fmt.Errorf("creating bead %s: %w", id, err)
	}
	fmt.Println(id)
	return nil
}

// beadCreateTargetDir picks the directory whose beads database receives the bead.
// An explicit --rig must exist. Without one, the rig you are standing in is used
// when it has its own beads database, otherwise the town database.
func beadCreateTargetDir(townRoot, rig string) (string, error) {
	if rig != "" {
		dir := filepath.Join(townRoot, rig)
		if _, err := os.Stat(filepath.Join(dir, ".beads")); err != nil {
			return "", fmt.Errorf("rig %q has no beads database at %s: %w", rig, dir, err)
		}
		return dir, nil
	}
	if inferred, err := inferRigFromCwd(townRoot); err == nil {
		dir := filepath.Join(townRoot, inferred)
		if _, statErr := os.Stat(filepath.Join(dir, ".beads")); statErr == nil {
			return dir, nil
		}
	}
	return townRoot, nil
}

// beadExists reports whether id is in the beads database at dir. Only bd's
// "not found" answer counts as absent; any other failure is returned so a
// broken database never reads as a free ID.
func beadExists(dir, id string) (bool, error) {
	out, err := BdCmd("show", id).Dir(dir).CombinedOutput()
	if err == nil {
		return true, nil
	}
	if bdShowSaysMissing(out, id) {
		return false, nil
	}
	return false, fmt.Errorf("bd show %s: %w: %s", id, err, strings.TrimSpace(string(out)))
}

// bdShowSaysMissing matches bd's exact answer for an absent ID ("Issue <id> not
// found"), not the looser "not found" heuristic used elsewhere, so a missing
// binary or database never reads as a free ID.
func bdShowSaysMissing(output []byte, id string) bool {
	return bytes.Contains(output, []byte("Issue "+id+" not found"))
}
