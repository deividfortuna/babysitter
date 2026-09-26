package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/store"
)

type notificationsOutput httpd.NotificationList

func (n notificationsOutput) writeText(out io.Writer) error {
	if len(n.Notifications) == 0 {
		_, err := fmt.Fprintln(out, "No notifications.")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, item := range n.Notifications {
		mark := "*"
		if item.ReadAt != nil {
			mark = " "
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", mark, item.ID, item.CreatedAt.Local().Format(time.RFC3339),
			item.Kind, item.Title, item.Body)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "\n%d unread\n", n.UnreadCount)
	return err
}

type notificationsReadOutput httpd.NotificationsRead

func (n notificationsReadOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "%d unread\n", n.UnreadCount)
	return err
}

func newNotificationsCmd(opts *options) *cobra.Command {
	var dataDirFlag string
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Read the notifications of the running daemon",
		Long: fmt.Sprintf(`The notifications the daemon recorded: what happened on the pull
requests it watches, and what an agent asked you for with 'notify'.

They are the same notifications the desktop app shows. The history keeps
the newest %d rows: a row of a watch goes when that watch is removed,
and the oldest fall out of the bottom as new ones arrive.`, store.DefaultNotificationCap),
	}
	cmd.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json")
	cmd.AddCommand(newNotificationsListCmd(opts, &dataDirFlag), newNotificationsReadCmd(opts, &dataDirFlag))
	return cmd
}

func newNotificationsListCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		unread bool
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print the notifications, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			q := url.Values{}
			if unread {
				q.Set("status", "unread")
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			path := "/notifications"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			var list httpd.NotificationList
			if err := c.get(cmd.Context(), path, &list); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), notificationsOutput(list))
		},
	}
	cmd.Flags().BoolVar(&unread, "unread", false, "only the notifications you have not seen")
	cmd.Flags().IntVar(&limit, "limit", 0, "how many rows at most")
	return cmd
}

func newNotificationsReadCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "read [id]...",
		Short: "Mark notifications as seen",
		Long: `Mark notifications as seen. With no id it marks every unread one.

The desktop app marks a notification when you click its banner or its
row, so this is for a terminal that runs without the app.`,
		Example: `  babysitter notifications read
  babysitter notifications read 12 13`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			var out httpd.NotificationsRead
			if err := c.post(cmd.Context(), "/notifications/read", httpd.ReadNotificationsRequest{IDs: ids}, &out); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), notificationsReadOutput(out))
		},
	}
}

func parseIDs(args []string) ([]int64, error) {
	if len(args) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(args))
	for _, arg := range args {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%q is not a notification id", arg)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (o *options) postNotification(ctx context.Context, dataDir string, req httpd.NewNotificationRequest) (httpd.Notification, error) {
	c, err := o.daemonClient(dataDir)
	if err != nil {
		return httpd.Notification{}, err
	}
	var out httpd.Notification
	if err := c.post(ctx, "/notifications", req, &out); err != nil {
		return httpd.Notification{}, err
	}
	return out, nil
}

var errNotRecorded = errors.New("the daemon holds no such notification")

func (o *options) recordedNotification(ctx context.Context, dataDir string, req httpd.NewNotificationRequest, since time.Time) (httpd.Notification, error) {
	c, err := o.daemonClient(dataDir)
	if err != nil {
		return httpd.Notification{}, err
	}
	var list httpd.NotificationList
	if err := c.get(ctx, "/notifications?limit=1", &list); err != nil {
		return httpd.Notification{}, err
	}
	if len(list.Notifications) == 0 {
		return httpd.Notification{}, errNotRecorded
	}
	row := list.Notifications[0]
	sameText := row.Title == strings.TrimSpace(req.Title) && strings.HasSuffix(row.Body, strings.TrimSpace(req.Body))
	if !sameText || row.CreatedAt.Before(since.Truncate(time.Second)) {
		return httpd.Notification{}, errNotRecorded
	}
	return row, nil
}
