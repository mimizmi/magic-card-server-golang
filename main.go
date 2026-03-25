package main

import (
	"log/slog"
	"os"

	"echo/internal/game"
	"echo/internal/matchmaking"
	"echo/internal/network"
	"echo/internal/player"
	"echo/internal/room"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// ── 依赖初始化（顺序有意义，被依赖的先初始化）──────────────
	playerMgr := player.NewManager()
	roomMgr := room.NewManager()
	queue := matchmaking.NewQueue(roomMgr)

	// 玩家断线时，从匹配队列中移除（防止匹配到已离线的玩家）
	playerMgr.OnDisconnect(func(p *player.Player) {
		queue.Dequeue(p.ID)
	})

	// ── 消息路由注册 ───────────────────────────────────────────
	router := network.NewRouter()

	// 匹配层：登录、入队、退队
	mmHandler := matchmaking.NewHandler(playerMgr, queue, roomMgr)
	mmHandler.RegisterAll(router)

	// 游戏层：角色选择、行动阶段所有操作
	gameHandler := game.NewHandler(playerMgr, roomMgr)
	gameHandler.RegisterAll(router)

	// 房间创建时，自动为该房间创建并启动游戏引擎
	roomMgr.OnRoomCreated(gameHandler.OnRoomCreated)

	// ── 启动服务器 ─────────────────────────────────────────────
	srv := network.NewServer("0.0.0.0:43966", router)
	if err := srv.Start(); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}