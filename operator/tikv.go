package operator

import (
	"fmt"
	"strings"
	"sync"

	"k8s.io/client-go/pkg/api/v1"

	"time"

	"errors"

	"sort"

	"github.com/astaxie/beego/logs"
	"github.com/ffan/tidb-operator/pkg/util/k8sutil"
	"github.com/ffan/tidb-operator/pkg/util/pdutil"
	"github.com/ffan/tidb-operator/pkg/util/retryutil"
	"github.com/ghodss/yaml"
	"github.com/tidwall/gjson"
)

const (
	defaultTikvPort = 20160
)

var (
	errMultipleStoresOneAddress = errors.New("multiple stores one address")
)

func (tk *Tikv) upgrade() (err error) {
	if len(tk.Stores) < 1 {
		return nil
	}
	if tk.Db.Status.Phase < PhaseTikvStarted {
		return fmt.Errorf("the db %s tikv unavailable", tk.Db.Metadata.Name)
	}

	var (
		upgraded bool
		count    int
	)

	e := NewEvent(tk.Db.Metadata.Name, "tikv", "upgrate")
	defer func() {
		// have upgrade
		if err != nil {
			tk.UpgradeState = upgradeFailed
		} else if count > 0 {
			tk.UpgradeState = upgradeOk
		}
		if count > 0 || err != nil {
			if uerr := tk.Db.update(); uerr != nil {
				logs.Error("update tidb error: %v", uerr)
			}
			e.Trace(err, fmt.Sprintf("upgrate tikv to version %s", tk.Version))
		}
	}()

	for _, st := range tk.Stores {
		upgraded, err = upgradeOne(st.Name, fmt.Sprintf("%s/tikv:%s", imageRegistry, tk.Version), tk.Version)
		if err != nil {
			return err
		}
		if upgraded {
			count++
			time.Sleep(reconcileInterval)
		}
	}
	return nil
}

func (tk *Tikv) install() (err error) {
	e := NewEvent(tk.Db.Metadata.Name, "tikv", "install")
	tk.Db.Status.Phase = PhaseTikvPending
	tk.Db.update()
	tk.Stores = make(map[string]*Store)
	defer func() {
		ph := PhaseTikvStarted
		if err != nil {
			ph = PhaseTikvStartFailed
		}
		tk.Db.Status.Phase = ph
		if uerr := tk.Db.update(); uerr != nil {
			logs.Error("update tidb error: %v", uerr)
		}
		e.Trace(err, fmt.Sprintf("Install tikv pods with replicas desire: %d, running: %d on k8s", tk.Spec.Replicas, tk.AvailableReplicas))
	}()
	for r := 1; r <= tk.Spec.Replicas; r++ {
		tk.Member++
		if err = tk._install(); err != nil {
			return err
		}
	}
	return err
}

func (tk *Tikv) _install() (err error) {
	tk.cur = fmt.Sprintf("tikv-%s-%03d", tk.Db.Metadata.Name, tk.Member)
	tk.Stores[tk.cur] = &Store{}
	tk.ReadyReplicas++
	if err = tk.createPod(); err != nil {
		return err
	}
	if err = tk.waitForOk(); err != nil {
		return err
	}
	tk.AvailableReplicas++
	return nil
}

func (tk *Tikv) createPod() (err error) {
	var j []byte
	if j, err = tk.toJSONTemplate(tikvPodYaml); err != nil {
		return err
	}
	var pod *v1.Pod
	if pod, err = k8sutil.CreateAndWaitPodByJSON(j, waitPodRuningTimeout); err != nil {
		return err
	}
	s := tk.Stores[tk.cur]
	s.Name = tk.cur
	s.Address = fmt.Sprintf("%s:%d", pod.Status.PodIP, defaultTikvPort)
	s.Node = pod.Spec.NodeName
	return nil
}

func (tk *Tikv) toJSONTemplate(temp string) ([]byte, error) {
	r := strings.NewReplacer(
		"{{version}}", tk.Spec.Version,
		"{{cpu}}", fmt.Sprintf("%v", tk.Spec.CPU),
		"{{mem}}", fmt.Sprintf("%v", tk.Spec.Mem),
		"{{capacity}}", fmt.Sprintf("%v", tk.Spec.Capatity),
		"{{tidbdata_volume}}", fmt.Sprintf("%v", tk.Spec.Volume),
		"{{id}}", fmt.Sprintf("%03v", tk.Member),
		"{{registry}}", imageRegistry,
		"{{cell}}", tk.Db.Metadata.Name,
		"{{namespace}}", getNamespace())
	str := r.Replace(temp)
	j, err := yaml.YAMLToJSON([]byte(str))
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (tk *Tikv) waitForOk() (err error) {
	interval := 3 * time.Second
	err = retryutil.Retry(interval, int(waitTidbComponentAvailableTimeout/(interval)), func() (bool, error) {
		j, err := pdutil.PdStoresGet(tk.Db.Pd.OuterAddresses[0])
		if err != nil {
			logs.Error("get stores by pd API: %v", err)
			return false, nil
		}
		ret := gjson.Get(j, "count")
		if ret.Int() < 1 {
			logs.Warn("current stores count: %d", ret.Int())
			return false, nil
		}
		// 获取online的tikv
		s := tk.Stores[tk.cur]
		ret = gjson.Get(j, fmt.Sprintf("stores.#[store.address==%s]#.store.id", s.Address))
		if ret.Type == gjson.Null {
			logs.Warn("cannt get store[%s]", tk.Stores[tk.cur].Address)
			return false, nil
		}
		if len(ret.Array()) != 1 {
			logs.Error("get multiple store by address[%s]", tk.Stores[tk.cur].Address)
			return false, errMultipleStoresOneAddress
		}
		s.ID = int(ret.Array()[0].Int())
		s.State = StoreOnline
		return true, nil
	})
	if err != nil {
		logs.Error("wait tikv %s available: %v", tk.cur, err)
	} else {
		logs.Info("tikv %s ok", tk.cur)
	}
	return err
}

func (tk *Tikv) IsBuried(s *Store) (bool, error) {
	j, err := pdutil.PdStoreIDGet(tk.Db.Pd.OuterAddresses[0], s.ID)
	if err != nil {
		return false, err
	}
	ret := gjson.Get(j, "store.state")
	if ret.Type == gjson.Null {
		return false, fmt.Errorf("cannt get store[%s] state", s.Name)
	}
	return StoreStatus(ret.Int()) == StoreTombstone, nil
}

func (tk *Tikv) uninstall() (err error) {
	cell := tk.Db.Metadata.Name
	defer func() {
		tk.Stores = nil
		tk.Member = 0
		tk.cur = ""
		tk.AvailableReplicas = 0
		tk.ReadyReplicas = 0
		if err == nil {
			err = tk.Db.update()
		}
	}()
	if err = k8sutil.DeletePodsBy(cell, "tikv"); err != nil {
		return err
	}
	return err
}

func (db *Db) scaleTikvs(replica int, wg *sync.WaitGroup) {
	if replica < 1 {
		return
	}
	kv := db.Tikv
	if replica == kv.Spec.Replicas {
		return
	}
	wg.Add(1)
	go func() {
		db.Lock()
		defer func() {
			db.Unlock()
			wg.Done()
		}()
		var err error
		e := NewEvent(db.Metadata.Name, "tikv", "scale")
		defer func(r int) {
			if err != nil {
				db.Status.ScaleState |= tikvScaleErr
			}
			db.update()
			e.Trace(err, fmt.Sprintf(`Scale tikv "%s" replica: %d->%d`, db.Metadata.Name, r, replica))
		}(kv.Spec.Replicas)
		switch n := replica - kv.Spec.Replicas; {
		case n > 0:
			err = kv.increase(n)
		case n < 0:
			err = kv.decrease(-n)
		}
	}()
}

func (tk *Tikv) decrease(replicas int) (err error) {
	if (tk.Spec.Replicas - replicas) < 3 {
		return fmt.Errorf("the replicas of tikv must more than %d", 3)
	}
	if replicas*3 > tk.Spec.Replicas {
		return fmt.Errorf("each scale can not be less than one-third")
	}
	logs.Info("start decrement scale tikv pod count: %d", replicas)

	var names []string
	for key := range tk.Stores {
		names = append(names, key)
	}

	sort.Strings(names)
	for i := 0; i <= replicas; i++ {
		name := names[i]
		if err = pdutil.PdStoreDelete(tk.Db.Pd.OuterAddresses[0], tk.Stores[name].ID); err != nil {
			return err
		}
		tk.Stores[name].State = 1
		tk.ReadyReplicas--
		logs.Warn("delete tikv %s store %d", name, tk.Stores[name].ID)
	}

	return nil
}

func (tk *Tikv) increase(replicas int) (err error) {
	md := getCachedMetadata()
	if (replicas + tk.Spec.Replicas) > md.Spec.Tikv.Max {
		return fmt.Errorf("the replicas of tikv exceeds max %d", md.Spec.Tikv.Max)
	}
	if replicas > tk.Spec.Replicas*2 {
		return fmt.Errorf("each scale can not exceed 2 times")
	}
	logs.Info("start increment scale tikv pod count: %d", replicas)
	for i := 0; i <= replicas; i++ {
		tk.Member++
		if err = tk._install(); err != nil {
			return err
		}
	}
	logs.Info("end incrementally scale tikv %s pod desire: %d, available: %d", tk.Db.Metadata.Name, tk.Spec.Replicas, tk.AvailableReplicas)
	return err
}

func isOkTikv(cell string) bool {
	if db, err := GetDb(cell); err != nil ||
		db == nil || db.Status.Phase < PhaseTikvStarted || db.Status.Phase > PhaseTidbInited {
		return false
	}
	return true
}

func (tk *Tikv) isNil() bool {
	return tk.Spec.Replicas < 1
}
