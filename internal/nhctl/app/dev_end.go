/*
Copyright 2020 The Nocalhost Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"fmt"
	"nocalhost/internal/nhctl/syncthing/terminate"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"nocalhost/pkg/nhctl/log"
)

func (a *Application) StopAllPortForward(svcName string) error {
	pidList := a.GetSvcProfileV2(svcName).PortForwardPidList
	killPidList := make([]string, 0)
	for _, v := range pidList {
		killPidList = append(killPidList, strings.Split(v, "-")[1])
	}
	if len(killPidList) == 0 {
		return errors.New("no port-forward pid found")
	}

	for _, killPid := range killPidList {
		pid, err := strconv.Atoi(killPid)
		if err != nil {
			log.WarnE(err, err.Error())
			continue
		}
		_ = terminate.Terminate(pid, true, "port-forward")
	}

	// Clean up port-forward status
	a.GetSvcProfileV2(svcName).DevPortList = make([]string, 0)
	//_ = a.DeleteDevPortList(svcName, killPortList)
	// set portForwardStatusList
	a.GetSvcProfileV2(svcName).PortForwardStatusList = make([]string, 0)
	//_ = a.DeletePortForwardStatusList(svcName, killPortList)
	// set portForwardPidList
	a.GetSvcProfileV2(svcName).PortForwardPidList = make([]string, 0)
	//_ = a.DeletePortForwardPidList(svcName, killPortList)
	return a.SaveProfile()
}

// port format 8080:80
func (a *Application) StopPortForwardByPort(svcName, port string) error {
	var err error
	pidList := a.GetSvcProfileV2(svcName).PortForwardPidList
	killPid := ""
	var killPortList []string
	if len(pidList) > 0 {
		for _, v := range pidList {
			getPort := strings.Split(v, "-")[0]
			if port == getPort {
				// get port-forward pid
				killPid = strings.Split(v, "-")[1]
			}
		}
		if killPid == "" {
			err := errors.New("can not find port-forward pid")
			return err
		}
		// get all devPorts, they share same pid of port-forward
		for _, v := range pidList {
			portPid := strings.Split(v, "-")
			port := portPid[0]
			pid := portPid[1]
			if pid == killPid {
				killPortList = append(killPortList, port)
			}
		}
	}
	// set devPortList
	if len(killPortList) == 0 {
		// if run here that means portForwardPidList doesn't has record
		// but portForwardStatusList or devPortList has record, it should get pid of listen port
		killPortList = append(killPortList, port)
		// find pid of port
		// TODO kill PID anyway
		// findLocalPortOfPid := strings.Split(port, ":")[0]
		// find this
		// linux: lsof -i :8080 -a -c nhctl -t
		// windows:
	}

	// killPid and killPortList
	if killPid != "" {
		pid, err := strconv.Atoi(killPid)
		if err != nil {
			err := errors.New("convert port-forward pid fail")
			return err
		}
		err = terminate.Terminate(pid, true, "port-forward")
		if err != nil {
			log.Warn(err.Error())
		}
	}
	// ignore terminate status and delete port-forward list anyway
	err = a.DeleteDevPortList(svcName, killPortList)
	if err != nil {
		log.Warn(err.Error())
	}
	// set portForwardStatusList
	err = a.DeletePortForwardStatusList(svcName, killPortList)
	if err != nil {
		log.Warn(err.Error())
	}
	// set portForwardPidList
	err = a.DeletePortForwardPidList(svcName, killPortList)
	if err != nil {
		log.Warn(err.Error())
	}
	return err
}

func (a *Application) stopSyncProcessAndCleanPidFiles(svcName string) error {
	var err error
	fileSyncOps := &FileSyncOptions{}
	devStartOptions := &DevStartOptions{}

	newSyncthing, err := a.NewSyncthing(svcName, devStartOptions.LocalSyncDir, fileSyncOps.SyncDouble)
	if err != nil {
		log.Warnf("Failed to start syncthing process: %s", err.Error())
		return err
	}

	// read and clean up pid file
	portForwardPid, portForwardFilePath, err := a.GetBackgroundSyncPortForwardPid(svcName, false)
	if err != nil {
		log.Warn("Failed to get background port-forward pid file, ignored")
	}
	if portForwardPid != 0 {
		err = newSyncthing.Stop(portForwardPid, portForwardFilePath, "port-forward", true)
		if err != nil {
			log.Warnf("Failed stop port-forward progress pid %d, please run `kill -9 %d` by manual, err: %s\n", portForwardPid, portForwardPid, err)
		}
	}

	// read and clean up pid file
	syncthingPid, syncThingPath, err := a.GetBackgroundSyncThingPid(svcName, false)
	if err != nil {
		log.Warn("Failed to get background syncthing pid file, ignored")
	}
	if syncthingPid != 0 {
		err = newSyncthing.Stop(syncthingPid, syncThingPath, "syncthing", true)
		if err != nil {
			if runtime.GOOS == "windows" {
				// in windows, it will raise a "Access is denied" err when killing progress, so we can ignore this err
				fmt.Printf("attempt to terminate syncthing process(pid: %d), you can run `tasklist | findstr %d` to make sure process was exited\n", portForwardPid, portForwardPid)
			} else {
				log.Warnf("Failed to terminate syncthing process(pid: %d), please run `kill -9 %d` manually, err: %s\n", portForwardPid, portForwardPid, err)
			}
		}
	}

	if err == nil { // none of them has error
		fmt.Printf("background port-forward process: %d and  syncthing process: %d terminated.\n", portForwardPid, syncthingPid)
	}

	// end dev port background port forward process from profile
	//onlyPortForwardPid, onlyPortForwardFilePath, err := a.GetBackgroundOnlyPortForwardPid(svcName, false)
	//if err != nil {
	//	log.Info("No dev port-forward pid file found, ignored.")
	//}
	//if onlyPortForwardPid != 0 {
	//	err = newSyncthing.Stop(onlyPortForwardPid, onlyPortForwardFilePath, "port-forward", true)
	//	if err != nil {
	//		log.Infof("Failed to terminate dev port-forward process(pid %d), please run `kill -9 %d` manually", onlyPortForwardPid, onlyPortForwardPid)
	//	}
	//}

	devPortsList := a.GetSvcProfileV2(svcName).DevPortList
	if len(devPortsList) > 0 {
		for _, v := range devPortsList {
			err := a.StopPortForwardByPort(svcName, v)
			if err == nil {
				fmt.Printf("dev port-forward: %s has been ended\n", v)
			}
		}
	}

	// Clean up secret
	svcProfile := a.GetSvcProfileV2(svcName)
	if svcProfile.SyncthingSecret != "" {
		log.Debugf("Cleaning up secret %s", svcProfile.SyncthingSecret)
		err = a.client.DeleteSecret(svcProfile.SyncthingSecret)
		if err != nil {
			log.WarnE(err, "Failed to clean up syncthing secret")
		} else {
			svcProfile.SyncthingSecret = ""
		}
	}

	// set profile status
	// set port-forward port and ignore result
	// err = a.SetSyncthingPort(svcName, 0, 0, 0, 0)
	err = a.SetSyncthingProfileEndStatus(svcName)
	return err
}

func (a *Application) Reset(svcName string) {
	var err error
	err = a.stopSyncProcessAndCleanPidFiles(svcName)
	if err != nil {
		if err != nil {
			log.Warnf("something incorrect occurs when stopping sync process: %s", err.Error())
		}
	}
	err = a.RollBack(context.TODO(), svcName, true)
	if err != nil {
		log.Warnf("something incorrect occurs when rolling back: %s", err.Error())
	}
	err = a.SetDevEndProfileStatus(svcName)
	if err != nil {
		log.Warnf("fail to update \"developing\" status")
	}
}

func (a *Application) EndDevelopMode(svcName string) error {
	var err error
	if !a.CheckIfSvcIsDeveloping(svcName) {
		return errors.New(fmt.Sprintf("\"%s\" is not in developing status", svcName))
	}

	log.Info("Ending devMode...")
	// end file sync
	log.Info("Terminating file sync process...")
	err = a.stopSyncProcessAndCleanPidFiles(svcName)
	if err != nil {
		log.WarnE(err, "Error occurs when stopping sync process")
		return err
	}

	// roll back workload
	log.Debug("Rolling back workload...")
	err = a.RollBack(context.TODO(), svcName, false)
	if err != nil {
		log.Error("Failed to rollback")
		return err
	}

	err = a.SetDevEndProfileStatus(svcName)
	if err != nil {
		log.Warn("Failed to update \"developing\" status")
		return err
	}
	return nil
}
