import { useState, useCallback } from "react";
import { apiJson } from "../api/client";
import type { RootCourseGroupInfo, GroupWithCount } from "../utils/levels";

export function useRootCourseGroups() {
  const [rootCourseGroups, setRootCourseGroups] = useState<RootCourseGroupInfo[]>([]);
  const [manageGroups, setManageGroups] = useState<GroupWithCount[]>([]);
  const [manageLoading, setManageLoading] = useState(false);
  const [managePage, setManagePage] = useState(0);
  const [newGroupName, setNewGroupName] = useState("");
  const [savingNewGroup, setSavingNewGroup] = useState(false);
  const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
  const [editingGroupName, setEditingGroupName] = useState("");
  const [savingEditGroup, setSavingEditGroup] = useState(false);

  const fetchRootCourseGroups = useCallback(async () => {
    const data = await apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" });
    setRootCourseGroups(data);
    return data;
  }, []);

  const fetchManageGroups = useCallback(async () => {
    setManageLoading(true);
    try {
      const data = await apiJson<GroupWithCount[]>("/api/v1/admin/root-course-groups", { method: "GET" });
      setManageGroups(data);
      setManagePage(0);
    } finally {
      setManageLoading(false);
    }
  }, []);

  const createGroup = useCallback(async (name: string): Promise<RootCourseGroupInfo> => {
    const created = await apiJson<RootCourseGroupInfo>("/api/v1/admin/root-course-groups", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    return created;
  }, []);

  const renameGroup = useCallback(async (id: string, name: string) => {
    await apiJson(`/api/v1/admin/root-course-groups/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name }),
    });
  }, []);

  const deleteGroup = useCallback(async (id: string) => {
    await apiJson(`/api/v1/admin/root-course-groups/${id}`, { method: "DELETE" });
  }, []);

  return {
    rootCourseGroups,
    setRootCourseGroups,
    manageGroups,
    setManageGroups,
    manageLoading,
    managePage,
    setManagePage,
    newGroupName,
    setNewGroupName,
    savingNewGroup,
    setSavingNewGroup,
    editingGroupId,
    setEditingGroupId,
    editingGroupName,
    setEditingGroupName,
    savingEditGroup,
    setSavingEditGroup,
    fetchRootCourseGroups,
    fetchManageGroups,
    createGroup,
    renameGroup,
    deleteGroup,
  };
}
