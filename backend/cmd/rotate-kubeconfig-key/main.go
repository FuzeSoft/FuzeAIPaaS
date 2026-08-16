
package main

import (
	"flag"
	"log"
	"os"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/storage"

	_ "github.com/mattn/go-sqlite3" 
)

func main() {
	var (
		dbPath  = flag.String("db", os.Getenv("DB_PATH"), "sqlite 数据库路径（必填，生产不得为 :memory:）")
		oldKey  = flag.String("old-key", os.Getenv("KUBECONFIG_ENC_KEY_OLD"), "旧主密钥（hex 64 字符，AES-256）")
		newKey  = flag.String("new-key", os.Getenv("KUBECONFIG_ENC_KEY"), "新主密钥（hex 64 字符，AES-256）")
		dryRun  = flag.Bool("dry-run", false, "仅统计待处理条数，不写库")
	)
	flag.Parse()

	if *dbPath == "" {
		log.Fatal("--db (或 DB_PATH) 必须指定")
	}
	if *oldKey == "" {
		log.Fatal("--old-key (或 KUBECONFIG_ENC_KEY_OLD) 必须指定")
	}
	if *newKey == "" {
		log.Fatal("--new-key (或 KUBECONFIG_ENC_KEY) 必须指定")
	}

	oldC, err := cipherFromHex(*oldKey)
	if err != nil {
		log.Fatalf("旧密钥非法: %v", err)
	}
	newC, err := cipherFromHex(*newKey)
	if err != nil {
		log.Fatalf("新密钥非法: %v", err)
	}
	if *oldKey == *newKey {
		log.Fatal("旧密钥与新密钥相同，无需轮转")
	}

	db, err := storage.NewSQLiteDBAt(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	store := storage.NewStorage(db)

	n, err := store.CountKubeConfigEnc()
	if err != nil {
		log.Fatalf("统计待处理密文失败: %v", err)
	}
	if *dryRun {
		log.Printf("[dry-run] 待轮转集群数: %d", n)
		return
	}
	if n == 0 {
		log.Printf("没有需要轮转的 kubeconfig 密文，无需操作")
		return
	}

	rotated, err := store.RotateKubeConfigKeys(oldC, newC)
	if err != nil {
		log.Fatalf("轮转失败（已回滚，数据库未改动）: %v", err)
	}
	log.Printf("密钥轮转完成：共重加密 %d 个集群的 kubeconfig", rotated)
}

func cipherFromHex(hexKey string) (*aes.Cipher, error) {
	k, err := aes.LoadMasterKey(func(string) string { return hexKey })
	if err != nil {
		return nil, err
	}
	return aes.NewCipher(k), nil
}