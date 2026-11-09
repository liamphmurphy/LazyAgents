"use client";

import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";
import { chatKeys, useChat } from "./hooks";
import { courseBuilding } from "../learning";

export type Course = Schemas["Course"];
export type CourseSummary = Schemas["CourseSummary"];
export type CourseLesson = Schemas["CourseLesson"];
export type CourseCheck = Schemas["CourseCheck"];
export type CourseOutline = Schemas["CourseOutline"];

const BUILD_POLL_MS = 4000;
const TUTOR_POLL_MS = 2000;

export const courseKeys = {
  all: ["courses"] as const,
  list: ["courses", "list"] as const,
  detail: (id: string) => ["courses", "detail", id] as const,
};

/** The person's modules; follows along while any is being built. */
export function useCourses() {
  return useQuery({
    queryKey: courseKeys.list,
    queryFn: () => unwrap(api.GET("/learn/courses")),
    refetchInterval: (q) => (q.state.data?.some((c) => courseBuilding(c.state)) ? BUILD_POLL_MS : false),
  });
}

export function useCourse(id: string) {
  return useQuery({
    queryKey: courseKeys.detail(id),
    queryFn: () => unwrap(api.GET("/learn/courses/{id}", { params: { path: { id } } })),
    refetchInterval: (q) => (q.state.data && courseBuilding(q.state.data.summary.state) ? BUILD_POLL_MS : false),
  });
}

function useCourseInvalidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: courseKeys.all });
}

export function useCreateCourse() {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["CourseCreate"]) => unwrap(api.POST("/learn/courses", { body })),
    onSettled: done,
  });
}

export function useEditCourseOutline(id: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["CourseOutlineEdit"]) => unwrap(api.PUT("/learn/courses/{id}/outline", { params: { path: { id } }, body })),
    onSettled: done,
  });
}

export function useApproveCourse(id: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["CourseApprove"]) => unwrap(api.POST("/learn/courses/{id}/approve", { params: { path: { id } }, body })),
    onSettled: done,
  });
}

export function useNextPart(id: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/learn/courses/{id}/next-part", { params: { path: { id } } })),
    onSettled: done,
  });
}

export function useStopCourse(id: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/learn/courses/{id}/stop", { params: { path: { id } } })),
    onSettled: done,
  });
}

export function useDeleteCourse() {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/learn/courses/{id}", { params: { path: { id } } })),
    onSettled: done,
  });
}

export function useAttemptCheck(courseId: string, checkId: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["CourseAttemptCreate"]) =>
      unwrap(api.POST("/learn/courses/{id}/checks/{checkId}/attempts", { params: { path: { id: courseId, checkId } }, body })),
    onSettled: done,
  });
}

export function useGradeAttempt(courseId: string) {
  const done = useCourseInvalidate();
  return useMutation({
    mutationFn: (v: { attemptId: string; correct: boolean }) =>
      unwrap(api.POST("/learn/courses/{id}/attempts/{attemptId}/grade", { params: { path: { id: courseId, attemptId: v.attemptId } }, body: { correct: v.correct } })),
    onSettled: done,
  });
}

/** The tutor's conversation, followed while it's answering. */
export function useTutor(courseId: string, chatId: string) {
  const qc = useQueryClient();
  const chat = useChat(chatId);
  const running = chat.data?.state === "running";
  const messages = useQuery({
    queryKey: chatKeys.messages(chatId),
    queryFn: () => unwrap(api.GET("/chats/{id}/messages", { params: { path: { id: chatId }, query: { limit: 200 } } })),
    refetchInterval: running ? TUTOR_POLL_MS : false,
  });
  // The reply lands as the run ends: one last read after it stops.
  const wasRunning = useRef(false);
  useEffect(() => {
    if (wasRunning.current && !running) void qc.invalidateQueries({ queryKey: chatKeys.messages(chatId) });
    wasRunning.current = running;
  }, [running, chatId, qc]);
  const send = useMutation({
    mutationFn: (v: { body: string; lesson: string }) => unwrap(api.POST("/learn/courses/{id}/tutor", { params: { path: { id: courseId } }, body: v })),
    onSettled: () => Promise.all([qc.invalidateQueries({ queryKey: chatKeys.detail(chatId) }), qc.invalidateQueries({ queryKey: chatKeys.messages(chatId) })]),
  });
  return { messages, send, running };
}
