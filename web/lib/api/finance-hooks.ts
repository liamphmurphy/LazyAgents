"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, postForm, unwrap, type Schemas } from "./client";

// ---- The Finance app: one person's accounts, imports and categories.

export type FinanceOverview = Schemas["FinanceOverview"];
export type FinanceAccount = Schemas["FinanceAccount"];
export type FinanceReviewItem = Schemas["FinanceReviewItem"];
export type FinanceImportResult = Schemas["FinanceImportResult"];

export const financeKeys = {
  all: ["finance"] as const,
  overview: ["finance", "overview"] as const,
};

/** The whole dashboard, twelve months of it; the chart shows the last six or all twelve. */
export function useFinance() {
  return useQuery({ queryKey: financeKeys.overview, queryFn: () => unwrap(api.GET("/finance", { params: { query: { months: 12 } } })) });
}

function useFinanceInvalidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: financeKeys.all });
}

/** The dashboard refreshes when the person closes the import dialog (useRefreshFinance), so its numbers land together. */
export function useImportStatement() {
  return useMutation({
    mutationFn: (v: { file: File; accountId?: string; flipSign?: boolean }) => {
      const form = new FormData();
      form.append("file", v.file);
      if (v.accountId) form.append("account_id", v.accountId);
      if (v.flipSign) form.append("flip_sign", "true");
      return postForm<FinanceImportResult>("/finance/imports", form);
    },
  });
}

export function useRefreshFinance() {
  return useFinanceInvalidate();
}

export function useCreateFinanceAccount() {
  const done = useFinanceInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["NewFinanceAccount"]) => unwrap(api.POST("/finance/accounts", { body })),
    onSettled: () => void done(),
  });
}

export function useSetFinanceBalance() {
  const done = useFinanceInvalidate();
  return useMutation({
    mutationFn: (v: { accountId: string; body: Schemas["FinanceBalanceInput"] }) =>
      unwrap(api.PUT("/finance/accounts/{accountId}/balance", { params: { path: { accountId: v.accountId } }, body: v.body })),
    onSettled: () => void done(),
  });
}

/**
 * Categorize a transaction in review. The row leaves the queue at once (it animates out),
 * then the dashboard refetches for the rule's effect on totals.
 */
export function useCategorize() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; category: string; saveRule?: boolean }) =>
      unwrap(
        api.PUT("/finance/transactions/{transactionId}/category", {
          params: { path: { transactionId: v.id } },
          body: { category: v.category, save_rule: v.saveRule ?? true },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: financeKeys.all }),
  });
}
