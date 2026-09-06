<script lang="ts">
  import { onMount } from "svelte";
  import { apiRequest } from "$lib/api";
  import type { InboxState } from "$lib/store.svelte";
  import type { WorkspaceState } from "$lib/workspace.svelte";
  import {
    CheckIcon,
    PlusIcon,
    TrashIcon,
  } from "@fvilers/heroicons-svelte/24/outline";

  let {
    inbox,
    workspace,
    onStatus,
  }: {
    inbox?: InboxState;
    workspace?: WorkspaceState;
    onStatus: (kind: "success" | "error", message: string) => void;
  } = $props();

  let accountSlug = $state("");
  let savingSlug = $state(false);
  let teamUsers = $state<any[]>([]);
  let showAddUserModal = $state(false);
  let newUsername = $state("");
  let newPassword = $state("");
  let newRole = $state<"agent" | "manager">("agent");
  let addingUser = $state(false);
  let userModalError = $state("");
  let createdUserResult = $state<{
    username: string;
    plaintextPassword?: string;
    role: string;
  } | null>(null);
  let copiedPassword = $state(false);
  let resetUserTarget = $state<any | null>(null);
  let resetNewPassword = $state("");
  let resettingPassword = $state(false);
  let resetPasswordError = $state("");
  let deleteUserTarget = $state<any | null>(null);
  let deletingUser = $state(false);

  onMount(() => {
    teamUsers = workspace?.users ?? [];
    void loadAccountSlug();
  });

  function closeModal() {
    showAddUserModal = false;
    createdUserResult = null;
    deleteUserTarget = null;
    resetUserTarget = null;
    userModalError = "";
    resetPasswordError = "";
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") closeModal();
  }

  async function refreshUsers() {
    if (workspace) {
      await workspace.refreshUsers();
      teamUsers = workspace.users;
    } else {
      teamUsers = await apiRequest("/workspace/users");
    }
  }

  async function loadAccountSlug() {
    try {
      const response = await apiRequest("/workspace/account/slug");
      if (response?.slug) accountSlug = response.slug;
    } catch {
      // The login prefix is optional for older workspaces.
    }
  }

  async function saveSlug() {
    if (!accountSlug.trim()) return onStatus("error", "Slug cannot be empty.");
    savingSlug = true;
    try {
      await apiRequest("/workspace/account/slug", {
        method: "PUT",
        body: { slug: accountSlug.trim() },
      });
      onStatus("success", "Workspace slug updated.");
    } catch (error) {
      onStatus(
        "error",
        error instanceof Error
          ? error.message
          : "Failed to update workspace slug.",
      );
    } finally {
      savingSlug = false;
    }
  }

  async function updateUserRole(userID: string, role: string) {
    try {
      await apiRequest(`/workspace/users/${userID}/role`, {
        method: "PUT",
        body: { role },
      });
      await refreshUsers();
      onStatus("success", "User role updated.");
    } catch (error) {
      onStatus(
        "error",
        error instanceof Error ? error.message : "Failed to update user role.",
      );
    }
  }

  function generatePassword() {
    const chars =
      "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%";
    return Array.from({ length: 12 }, () =>
      chars.charAt(Math.floor(Math.random() * chars.length)),
    ).join("");
  }

  async function addUser() {
    userModalError = "";
    if (!newUsername.trim() || !newPassword.trim()) {
      userModalError = !newUsername.trim()
        ? "Username is required."
        : "Password is required.";
      return;
    }
    addingUser = true;
    try {
      const response = await apiRequest("/workspace/users", {
        method: "POST",
        body: {
          username: newUsername.trim(),
          password: newPassword.trim(),
          role: newRole,
        },
      });
      createdUserResult = {
        username: response.username || newUsername.trim(),
        role: response.role || newRole,
        plaintextPassword: response.password || newPassword.trim(),
      };
      newUsername = "";
      newPassword = "";
      newRole = "agent";
      showAddUserModal = false;
      await refreshUsers();
    } catch (error) {
      userModalError =
        error instanceof Error ? error.message : "Failed to create user.";
    } finally {
      addingUser = false;
    }
  }

  async function resetPassword() {
    if (!resetUserTarget || !resetNewPassword.trim()) return;
    resettingPassword = true;
    resetPasswordError = "";
    try {
      await apiRequest(`/workspace/users/${resetUserTarget.id}/password`, {
        method: "PUT",
        body: { password: resetNewPassword.trim() },
      });
      createdUserResult = {
        username: resetUserTarget.username || resetUserTarget.email,
        role: resetUserTarget.role,
        plaintextPassword: resetNewPassword.trim(),
      };
      resetUserTarget = null;
      resetNewPassword = "";
      onStatus("success", "Password reset successfully.");
    } catch (error) {
      resetPasswordError =
        error instanceof Error ? error.message : "Failed to reset password.";
    } finally {
      resettingPassword = false;
    }
  }

  async function deleteUser() {
    if (!deleteUserTarget) return;
    deletingUser = true;
    try {
      await apiRequest(`/workspace/users/${deleteUserTarget.id}`, {
        method: "DELETE",
      });
      deleteUserTarget = null;
      await refreshUsers();
      onStatus("success", "User deleted and conversations unassigned.");
    } catch (error) {
      onStatus(
        "error",
        error instanceof Error ? error.message : "Failed to delete user.",
      );
    } finally {
      deletingUser = false;
    }
  }

  async function copyCredentials() {
    if (!createdUserResult?.plaintextPassword) return;
    const login = accountSlug
      ? `${accountSlug}-${createdUserResult.username}`
      : createdUserResult.username;
    try {
      await navigator.clipboard.writeText(
        `Login: ${login}\nPassword: ${createdUserResult.plaintextPassword}`,
      );
      copiedPassword = true;
      setTimeout(() => (copiedPassword = false), 2000);
    } catch {
      // Clipboard access can be unavailable in non-secure browser contexts.
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h2 class="text-base font-medium text-slate-900">Users & permissions</h2>
      <p class="text-xs text-slate-500 mt-0.5">
        Manage team members and their workspace access.
      </p>
    </div>
    <button
      onclick={() => {
        userModalError = "";
        newPassword = generatePassword();
        showAddUserModal = true;
      }}
      class="px-3.5 py-2 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-xl flex items-center gap-1.5 transition cursor-pointer shadow-xs"
      ><PlusIcon class="w-3.5 h-3.5" /><span>Add user</span></button
    >
  </div>
  <div
    class="p-4 bg-slate-50/80 border border-slate-200 rounded-2xl space-y-2.5"
  >
    <div class="flex items-center justify-between">
      <label
        for="settings-workspace-slug"
        class="text-xs font-medium text-slate-900">Workspace login prefix</label
      ><button
        type="button"
        onclick={saveSlug}
        disabled={savingSlug}
        class="px-3 py-1 bg-white border border-slate-200 hover:border-slate-300 rounded-lg text-xs font-medium text-slate-700 transition cursor-pointer disabled:opacity-50"
        >{savingSlug ? "Saving..." : "Update prefix"}</button
      >
    </div>
    <input
      id="settings-workspace-slug"
      type="text"
      bind:value={accountSlug}
      placeholder="company-prefix"
      class="w-full px-3 py-2 bg-white border border-slate-200 rounded-xl text-xs font-mono text-slate-900 focus:border-blue-600 focus:ring-1 focus:ring-blue-100 outline-none"
    />
    <div class="space-y-1 text-[11px] text-slate-500">
      <p>
        Team members log in with: <span
          class="font-mono font-medium text-slate-800 bg-white px-1.5 py-0.5 rounded border border-slate-200"
          >{accountSlug || "prefix"}-[username]</span
        >
      </p>
      <p>
        Agents only see their assigned leads. Managers see all workspace leads.
      </p>
    </div>
  </div>
  <div
    class="border border-slate-200 rounded-2xl overflow-hidden divide-y divide-slate-100 text-xs bg-white"
  >
    {#each teamUsers as user (user.id)}
      <div
        class="p-4 flex items-center justify-between hover:bg-slate-50/50 transition"
      >
        <div class="flex items-center gap-3 min-w-0">
          <div
            class="w-8 h-8 rounded-full bg-blue-100 text-blue-700 font-medium flex items-center justify-center text-xs shrink-0"
          >
            {(user.username || user.name || user.email).charAt(0).toUpperCase()}
          </div>
          <div class="min-w-0">
            <div class="font-medium text-slate-800 truncate">
              {user.username || user.name || user.email.split("@")[0]}
            </div>
            <div class="text-[11px] text-slate-400 font-mono truncate">
              {user.email ||
                (accountSlug
                  ? `${accountSlug}-${user.username}`
                  : user.username)}
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2.5 shrink-0">
          <select
            aria-label="Role for {user.username || user.email}"
            value={user.role}
            onchange={(event) =>
              updateUserRole(user.id, event.currentTarget.value)}
            class="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] font-medium capitalize text-slate-700 focus:border-blue-500 focus:outline-none cursor-pointer"
            ><option value="agent">Agent</option><option value="manager"
              >Manager</option
            ></select
          ><button
            type="button"
            class="px-2.5 py-1 text-[11px] font-medium text-slate-600 hover:text-slate-900 bg-slate-100 hover:bg-slate-200 rounded-lg transition cursor-pointer"
            onclick={() => {
              resetUserTarget = user;
              resetPasswordError = "";
              resetNewPassword = generatePassword();
            }}
            title="Reset Password">Reset password</button
          >{#if user.id !== inbox?.currentUser?.id}<button
              type="button"
              class="p-1 text-slate-400 hover:text-rose-600 rounded-lg hover:bg-rose-50 transition cursor-pointer"
              onclick={() => (deleteUserTarget = user)}
              title="Delete user"><TrashIcon class="w-4 h-4" /></button
            >{/if}
        </div>
      </div>
    {/each}
  </div>
</div>

{#if showAddUserModal}<div class="wf-modal-backdrop">
    <div
      class="wf-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-team-member-title"
    >
      <h3 id="add-team-member-title" class="text-sm font-medium text-slate-900">
        Add team member
      </h3>
      <div class="space-y-3.5 text-xs">
        <div class="space-y-1">
          <label for="newUsernameInput" class="font-medium text-slate-700"
            >Username</label
          ><input
            id="newUsernameInput"
            type="text"
            bind:value={newUsername}
            placeholder="e.g. john"
            class="wf-input"
          />
          <p class="text-[11px] text-slate-400">
            Login username: <span class="font-mono"
              >{accountSlug || "prefix"}-{newUsername || "[username]"}</span
            >
          </p>
        </div>
        <div class="space-y-1">
          <div class="flex items-center justify-between">
            <label for="newPasswordInput" class="font-medium text-slate-700"
              >Initial password</label
            ><button
              type="button"
              onclick={() => (newPassword = generatePassword())}
              class="text-[11px] text-blue-600 hover:underline cursor-pointer"
              >Generate</button
            >
          </div>
          <input
            id="newPasswordInput"
            type="text"
            bind:value={newPassword}
            placeholder="Password"
            class="wf-input font-mono"
          />
        </div>
        <div class="space-y-1">
          <label for="newRoleSelect" class="font-medium text-slate-700"
            >Role</label
          ><select id="newRoleSelect" bind:value={newRole} class="wf-select"
            ><option value="agent">Agent</option><option value="manager"
              >Manager</option
            ></select
          >
        </div>
        {#if userModalError}<p class="text-xs text-rose-600 font-medium">
            {userModalError}
          </p>{/if}
      </div>
      <div class="flex items-center justify-end gap-3 pt-2">
        <button
          onclick={closeModal}
          class="px-4 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 rounded-xl"
          >Cancel</button
        ><button
          onclick={addUser}
          disabled={addingUser || !newUsername.trim() || !newPassword.trim()}
          class="wf-button-primary disabled:opacity-50"
          >{addingUser ? "Saving..." : "Add user"}</button
        >
      </div>
    </div>
  </div>{/if}

{#if createdUserResult}<div class="wf-modal-backdrop">
    <div
      class="wf-modal max-w-md"
      role="dialog"
      aria-modal="true"
      aria-labelledby="user-credentials-title"
    >
      <div class="flex items-center gap-3">
        <div
          class="w-9 h-9 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center"
        >
          <CheckIcon class="w-5 h-5" />
        </div>
        <div>
          <h3
            id="user-credentials-title"
            class="text-sm font-medium text-slate-900"
          >
            User credentials
          </h3>
          <p class="text-xs text-slate-500">
            Save these credentials now. The system does not show this password
            again.
          </p>
        </div>
      </div>
      <div
        class="space-y-3 p-4 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono"
      >
        <div class="flex justify-between">
          <span>Login username:</span><span
            >{accountSlug
              ? `${accountSlug}-${createdUserResult.username}`
              : createdUserResult.username}</span
          >
        </div>
        <div class="flex justify-between">
          <span>Role:</span><span class="capitalize"
            >{createdUserResult.role}</span
          >
        </div>
        {#if createdUserResult.plaintextPassword}<div
            class="flex justify-between"
          >
            <span>Password:</span><span class="text-blue-700"
              >{createdUserResult.plaintextPassword}</span
            >
          </div>{/if}
      </div>
      <div class="flex justify-end gap-3">
        {#if createdUserResult.plaintextPassword}<button
            type="button"
            onclick={copyCredentials}
            class="px-4 py-2 text-xs bg-slate-100 rounded-xl"
            >{copiedPassword ? "Copied!" : "Copy credentials"}</button
          >{/if}<button onclick={closeModal} class="wf-button-primary px-4 py-2"
          >Done</button
        >
      </div>
    </div>
  </div>{/if}

{#if resetUserTarget}<div class="wf-modal-backdrop">
    <div
      class="wf-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="reset-user-password-title"
    >
      <h3
        id="reset-user-password-title"
        class="text-sm font-medium text-slate-900"
      >
        Reset User Password
      </h3>
      <p class="text-xs text-slate-500">
        Set a new password for <span class="font-medium"
          >{resetUserTarget.username || resetUserTarget.email}</span
        >.
      </p>
      <div class="space-y-1 text-xs">
        <div class="flex justify-between">
          <label for="resetPasswordInput">New Password</label><button
            type="button"
            onclick={() => (resetNewPassword = generatePassword())}
            class="text-blue-600">Generate</button
          >
        </div>
        <input
          id="resetPasswordInput"
          type="text"
          bind:value={resetNewPassword}
          class="wf-input font-mono"
        />{#if resetPasswordError}<p class="text-rose-600">
            {resetPasswordError}
          </p>{/if}
      </div>
      <div class="flex justify-end gap-3">
        <button onclick={closeModal} class="px-4 py-2 text-xs">Cancel</button
        ><button
          onclick={resetPassword}
          disabled={resettingPassword || !resetNewPassword.trim()}
          class="wf-button-primary disabled:opacity-50"
          >{resettingPassword ? "Updating..." : "Set Password"}</button
        >
      </div>
    </div>
  </div>{/if}

{#if deleteUserTarget}<div class="wf-modal-backdrop">
    <div
      class="wf-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-user-modal-title"
    >
      <h3
        id="delete-user-modal-title"
        class="text-sm font-medium text-slate-900"
      >
        Delete User
      </h3>
      <p class="text-xs text-slate-600">
        Are you sure you want to delete <span class="font-medium"
          >{deleteUserTarget.username || deleteUserTarget.email}</span
        >? Any conversations currently assigned to them will be unassigned. This
        action cannot be undone.
      </p>
      <div class="flex justify-end gap-3">
        <button onclick={closeModal} class="px-4 py-2 text-xs">Cancel</button
        ><button
          onclick={deleteUser}
          disabled={deletingUser}
          class="wf-button-danger disabled:opacity-50"
          >{deletingUser ? "Deleting..." : "Delete User"}</button
        >
      </div>
    </div>
  </div>{/if}
