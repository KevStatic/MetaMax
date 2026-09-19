package container

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// setupLUKSHome creates (or re-opens) a LUKS2-encrypted image at
// storageDir/vault.img and mounts the decrypted filesystem at
// storageDir/home. Returns the mount path.
//
// vaultKey is a hex string (64 chars = 32 bytes). If empty, a random key is
// generated — files are still encrypted, just not blockchain-gated.
func setupLUKSHome(storageDir, vaultKey string) (string, error) {
	imgPath := storageDir + "/vault.img"
	mountPath := storageDir + "/home"
	mapper := "comput3-" + randomMapperSuffix()
	keyfile := storageDir + "/vault.key"

	if err := os.MkdirAll(mountPath, 0o700); err != nil {
		return "", fmt.Errorf("mkdir mountpath: %w", err)
	}

	if vaultKey == "" {
		b := make([]byte, 32)
		rand.Read(b)
		vaultKey = hex.EncodeToString(b)
	}
	if err := os.WriteFile(keyfile, []byte(vaultKey), 0o600); err != nil {
		return "", fmt.Errorf("write keyfile: %w", err)
	}
	defer os.Remove(keyfile) // wipe after use

	loopDev, err := runOutput("losetup", "-f")
	if err != nil {
		return "", fmt.Errorf("losetup -f: %w", err)
	}
	loopDev = strings.TrimSpace(loopDev)

	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		log.Printf("[luks] creating 512MB encrypted volume at %s", imgPath)
		if err := luksRun("dd", "if=/dev/urandom", "of="+imgPath, "bs=1M", "count=512"); err != nil {
			return "", fmt.Errorf("dd: %w", err)
		}
		if err := luksRun("losetup", loopDev, imgPath); err != nil {
			return "", fmt.Errorf("losetup attach: %w", err)
		}
		if err := luksRun("cryptsetup", "luksFormat", "--batch-mode",
			"--key-file", keyfile, "--type", "luks2", loopDev); err != nil {
			luksRun("losetup", "-d", loopDev) //nolint:errcheck
			return "", fmt.Errorf("luksFormat: %w", err)
		}
		if err := luksRun("cryptsetup", "open", "--key-file", keyfile, loopDev, mapper); err != nil {
			luksRun("losetup", "-d", loopDev) //nolint:errcheck
			return "", fmt.Errorf("luksOpen: %w", err)
		}
		if err := luksRun("mkfs.ext4", "-q", "/dev/mapper/"+mapper); err != nil {
			luksRun("cryptsetup", "close", mapper) //nolint:errcheck
			luksRun("losetup", "-d", loopDev)      //nolint:errcheck
			return "", fmt.Errorf("mkfs: %w", err)
		}
		if err := luksRun("mount", "/dev/mapper/"+mapper, mountPath); err != nil {
			luksRun("cryptsetup", "close", mapper) //nolint:errcheck
			luksRun("losetup", "-d", loopDev)      //nolint:errcheck
			return "", fmt.Errorf("mount: %w", err)
		}
		os.Chmod(mountPath, 0o700)
		log.Printf("[luks] formatted and mounted at %s (mapper=%s loop=%s)", mountPath, mapper, loopDev)
	} else {
		log.Printf("[luks] re-opening existing vault at %s", imgPath)
		if err := luksRun("losetup", loopDev, imgPath); err != nil {
			return "", fmt.Errorf("losetup attach: %w", err)
		}
		if err := luksRun("cryptsetup", "open", "--key-file", keyfile, loopDev, mapper); err != nil {
			luksRun("losetup", "-d", loopDev) //nolint:errcheck
			return "", fmt.Errorf("luksOpen: %w", err)
		}
		if err := luksRun("mount", "/dev/mapper/"+mapper, mountPath); err != nil {
			luksRun("cryptsetup", "close", mapper) //nolint:errcheck
			luksRun("losetup", "-d", loopDev)      //nolint:errcheck
			return "", fmt.Errorf("mount: %w", err)
		}
		os.Chmod(mountPath, 0o700)
		log.Printf("[luks] re-mounted at %s (mapper=%s loop=%s)", mountPath, mapper, loopDev)
	}
	return mountPath, nil
}

// teardownLUKSHome unmounts and closes all LUKS devices associated with storageDir.
func teardownLUKSHome(storageDir string) {
	mountPath := storageDir + "/home"
	if err := luksRun("umount", "-l", mountPath); err != nil {
		log.Printf("[luks] umount %s: %v (continuing)", mountPath, err)
	}

	imgPath := storageDir + "/vault.img"
	out, _ := runOutput("losetup", "-j", imgPath)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		loopDev := strings.SplitN(line, ":", 2)[0]
		mappers, _ := runOutput("dmsetup", "ls", "--target", "crypt")
		for _, mline := range strings.Split(strings.TrimSpace(mappers), "\n") {
			fields := strings.Fields(mline)
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			status, _ := runOutput("dmsetup", "status", name)
			if strings.Contains(status, loopDev) || strings.HasPrefix(name, "comput3-") {
				luksRun("cryptsetup", "close", name) //nolint:errcheck
			}
		}
		luksRun("losetup", "-d", loopDev) //nolint:errcheck
	}
}

func luksRun(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w — %s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func randomMapperSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
