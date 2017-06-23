package mysqlutil

import (
	"os/exec"
	"strings"

	"fmt"

	"errors"

	"github.com/astaxie/beego/logs"
	"github.com/ffan/tidb-k8s/pkg/servenv"
)

var (
	binDir  = fmt.Sprintf("%s/pkg/mysqlutil/bin/", servenv.Root())
	checker = binDir + "checker -L error -host %s -port %d -user %s -password %s %s"

	errNoReplicationClientPri = errors.New("No replication client privilege or no super user")
)

// Migration mysql data to tidb
type Migration struct {
	Src     Mysql
	Dest    Mysql
	Tables  []string
	DataDir string

	ToggleSync bool
	NotifyAPI  string
}

// Check 预先检查 TiDB 是否能支持需要迁移的 table schema
func (m *Migration) Check() error {
	dsn := m.Src.Dsn()
	err := execMysqlCommand(dsn, "SELECT 1")
	if err != nil {
		return fmt.Errorf("ping mysql %s timeout: %v", dsn, err)
	}
	cmd := fmt.Sprintf(checker, m.Src.IP, m.Src.Port, m.Src.User, m.Src.Password, m.Src.Database)
	o, err := execShell(cmd)
	if err != nil {
		return fmt.Errorf("%s", o)
	}
	if m.ToggleSync {
		have, err := havePrivilege(dsn, m.Src.User, "REPLICATION CLIENT")
		if err != nil {
			return err
		}
		if !have {
			return errNoReplicationClientPri
		}
	}
	return nil
}

func execShell(cmd string) ([]byte, error) {
	logs.Info("Command is %s", cmd)
	parts := strings.Fields(cmd)
	head := parts[0]
	parts = parts[1:len(parts)]
	return exec.Command(head, parts...).CombinedOutput()
}