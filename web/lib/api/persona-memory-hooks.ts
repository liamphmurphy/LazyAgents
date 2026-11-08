"use client";

// ---- Personas' own memories: who each persona is, and the moments it shared with the person (the Library's Fridge).

import { useMemo } from "react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";
import { memoryKeys } from "./hooks";
import { usePersonas, type Persona } from "./persona-hooks";

/** The caller's personas by id, for labelling persona memories. */
export function usePersonaLookup(): Map<string, Persona> {
  const personas = usePersonas();
  return useMemo(() => new Map((personas.data?.personas ?? []).map((p) => [p.id, p])), [personas.data]);
}

/** Moments from every persona's own memory (or one persona's), newest first, 50 at a time. */
export function useMoments(personaId = "") {
  return useInfiniteQuery({
    // Under ["memories"], so memory edits and deletes refresh it.
    queryKey: [...memoryKeys.all, "moments", personaId] as const,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/memories", {
          params: {
            query: {
              type: "moment",
              sort: "created",
              ...(personaId ? { persona_id: personaId } : { about: "persona" }),
              ...(pageParam ? { offset: pageParam } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last, pages) => (last.has_more ? pages.reduce((n, p) => n + p.memories.length, 0) : undefined),
  });
}

/** Adds an entry by hand, e.g. a moment to a persona's own memory. */
export function useCreateMemory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["MemoryCreate"]) => unwrap(api.POST("/memories", { body })),
    onSettled: () => qc.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}
