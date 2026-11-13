"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";

/** Which engine makes images (Codex, Antigravity or whichever is connected), and which are connected now. */
export function useImageEngine() {
  return useQuery({ queryKey: ["image-engine"], queryFn: () => unwrap(api.GET("/image-engine")) });
}

export function useSetImageEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (engine: "auto" | "codex" | "antigravity") => unwrap(api.PUT("/image-engine", { body: { engine } })),
    onSuccess: (settings) => qc.setQueryData(["image-engine"], settings),
  });
}
