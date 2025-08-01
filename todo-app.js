// Lightweight .ics parser to extract VEVENT SUMMARY and DTSTART
/**
 * Parse raw ICS text and return array of {summary, date}
 * Only supports DTSTART in formats YYYYMMDD, YYYYMMDDThhmmss, or ...Z
 */
function parseICS(text) {
  // Unfold lines (RFC 5545 3.1) – remove CRLF followed by space / tab
  text = text.replace(/\r?\n[ \t]/g, "");
  const lines = text.split(/\r?\n/);
  const events = [];
  let current = null;
  for (const line of lines) {
    if (line.startsWith("BEGIN:VEVENT")) {
      current = { summary: null, date: null, rrule: null, isAllDay: false };
    } else if (line.startsWith("END:VEVENT")) {
      if (current) events.push(current);
      current = null;
    } else if (current) {
      if (line.startsWith("SUMMARY:")) {
        current.summary = line.substring(8).trim();
      } else if (line.startsWith("DTSTART")) {
        const [, value] = line.split(":");
        if (!value) continue;
        let date = null;
        const v = value.trim();
        if (/^\d{8}T\d{6}Z$/.test(v)) {
          date = new Date(v);
          current.isAllDay = false;
        } else if (/^\d{8}T\d{6}$/.test(v)) {
          const y = v.slice(0, 4),
            m = v.slice(4, 6),
            d = v.slice(6, 8);
          const H = v.slice(9, 11),
            M = v.slice(11, 13),
            S = v.slice(13, 15);
          date = new Date(
            Number(y),
            Number(m) - 1,
            Number(d),
            Number(H),
            Number(M),
            Number(S),
          );
          current.isAllDay = false;
        } else if (/^\d{8}$/.test(v)) {
          const y = v.slice(0, 4),
            m = v.slice(4, 6),
            d = v.slice(6, 8);
          date = new Date(Number(y), Number(m) - 1, Number(d));
          current.isAllDay = true;
        }
        current.date = date;
      } else if (line.startsWith("RRULE:")) {
        current.rrule = line.substring(6).trim();
      }
    }
  }

  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  // Filter events and map RRULE to recurrence fields
  return events
    .filter((ev) => {
      // Always keep events that have a recurrence rule.
      if (ev.rrule) return true;
      // If there's no date, keep it (it might be a timeless task).
      if (!ev.date) return true;

      if (ev.isAllDay) {
        // For all-day events, keep it if its date is today or in the future.
        return ev.date >= today;
      } else {
        // For timed events, only keep it if the date is not in the past.
        if (ev.summary.startsWith("Lucie")) console.log(ev, ev.date >= now);
        return ev.date >= now;
      }
    })
    .map((ev) => {
      if (ev.rrule) {
        const parts = Object.fromEntries(
          ev.rrule.split(";").map((p) => p.split("=")),
        );
        const freq = (parts.FREQ || "").toUpperCase();
        const interval = parseInt(parts.INTERVAL || "1", 10);
        let unit = null;
        switch (freq) {
          case "DAILY":
            unit = "day";
            break;
          case "WEEKLY":
            unit = "week";
            break;
          case "MONTHLY":
            unit = "month";
            break;
          case "YEARLY":
            unit = "year";
            break;
        }
        if (unit) {
          ev.recurrenceInterval = interval;
          ev.recurrenceUnit = unit;
        }
      }
      return ev;
    });
}
// Utility: format date as strict RFC3339 (no ms, always ends in Z)
//
function toRFC3339NoMillis(date) {
  const pad = (n) => n.toString().padStart(2, "0");
  return (
    date.getUTCFullYear() +
    "-" +
    pad(date.getUTCMonth() + 1) +
    "-" +
    pad(date.getUTCDate()) +
    "T" +
    pad(date.getUTCHours()) +
    ":" +
    pad(date.getUTCMinutes()) +
    ":" +
    pad(date.getUTCSeconds()) +
    "Z"
  );
}

async function subscribeToICS() {
  const url = prompt("Enter the URL of the ICS calendar to subscribe to:");
  if (!url) return;

  const projectName = prompt(
    "Enter the name of the project for the imported todos:",
  );
  if (!projectName) return;

  try {
    const response = await fetch("/api/subscribe_ics", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url: url, project_name: projectName }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to subscribe to ICS feed");
    }

    alert(
      "Successfully subscribed to ICS feed. The calendar will be updated periodically.",
    );
    await loadTodosByProject();
  } catch (error) {
    console.error("Error subscribing to ICS feed:", error);
    alert("Failed to subscribe to ICS feed. " + error.message);
  }
}

/**
 * Build a read-only project group that lists todos due in the next 6 days.
 * @param {Array} todos – the todos that are due soon
 */
async function loadUpcomingTodos(todos) {
  // Reuse full-featured loadProject to ensure consistent functionality
  const pseudoProject = { id: "thisweek", title: "This Week" };
  const group = await loadProject(pseudoProject, todos);

  // Make title read-only (remove click handlers and pointer events)
  group.querySelector(".project-title").style.pointerEvents = "none";

  // Remove elements that don't make sense for the special group
  group.querySelector(".delete-project-btn")?.remove();
  group.querySelector(".todo-form")?.remove();
  group.querySelector(".toggle-completed-btn")?.remove();
  group.querySelector(".completed-todos")?.remove();

  return group;
}

let editingProject = null;

// Convert URLs in plain text to clickable links
function linkify(text) {
  const urlPattern = /(https?:\/\/[^\s]+)/g;
  return text.replace(urlPattern, (url) => {
    const safe = url.replace(/"/g, "&quot;");
    return `<a href="${safe}" target="_blank" rel="noopener noreferrer">${url}</a>`;
  });
}

// Highlight hashtags
function hashtagify(text) {
  const hashtagPattern = /(#\w+)/g;
  return text.replace(hashtagPattern, '<span class="hashtag">$1</span>');
}

function startProjectEdit(element) {
  if (editingProject) return;

  const title = element.textContent;
  const textarea = document.createElement("textarea");
  textarea.value = title;
  textarea.className = "project-title";

  element.classList.add("editing");
  element.parentElement.insertBefore(textarea, element.nextSibling);

  // Set focus and select text
  textarea.focus();
  textarea.select();

  // Save reference for blur handling
  editingProject = {
    element: element,
    textarea: textarea,
  };

  // Handle blur
  textarea.addEventListener("blur", cancelProjectEdit);

  // Handle keydown events
  textarea.addEventListener("keydown", handleProjectEditKeydown);
}

function cancelProjectEdit() {
  if (!editingProject) return;

  const { element, textarea } = editingProject;
  element.classList.remove("editing");
  textarea.remove();
  editingProject = null;
}

async function saveProjectEdit() {
  if (!editingProject) return;

  const { element, textarea } = editingProject;
  const newTitle = textarea.value.trim();

  if (newTitle) {
    try {
      const response = await fetch(`/api/projects/${element.dataset.id}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          title: newTitle,
        }),
      });
      if (!response.ok) {
        throw new Error("Failed to update project");
      }
      element.textContent = newTitle;
    } catch (error) {
      console.error("Error updating project:", error);
      alert("Failed to update project. Please try again.");
      element.textContent = textarea.value;
    }
  }

  element.classList.remove("editing");
  textarea.remove();
  editingProject = null;
}

function handleProjectEditKeydown(e) {
  if (e.key === "Escape") {
    e.preventDefault();
    cancelProjectEdit();
  } else if (e.key === "Enter") {
    e.preventDefault();
    saveProjectEdit();
  }
}

async function deleteProject(id) {
  if (
    !confirm("Are you sure you want to delete this project and all its todos?")
  ) {
    return;
  }

  try {
    // Remove the expanded state from localStorage first
    localStorage.removeItem("completedExpanded_" + id);

    const response = await fetch(`/api/projects/${id}`, {
      method: "DELETE",
    });
    if (!response.ok) {
      throw new Error("Failed to delete project");
    }

    // Reload projects and todos
    await loadTodosByProject();
  } catch (error) {
    console.error("Error deleting project:", error);
    alert("Failed to delete project. Please try again.");
  }
}

// Add click handler for delete buttons
document.addEventListener("click", function (e) {
  const deleteBtn = e.target.closest(".delete-project-btn");
  if (deleteBtn) {
    const projectId = deleteBtn.dataset.id;
    deleteProject(projectId);
  }
});

// Menu toggle and dropdown handlers will be initialized after DOM is loaded
document.addEventListener("DOMContentLoaded", function () {
  // Toggle dropdown menu
  document.addEventListener("click", function (e) {
    const menuButton = document.getElementById("menuToggle");
    const dropdownMenu = document.getElementById("dropdownMenu");

    if (
      e.target === menuButton ||
      (menuButton && menuButton.contains(e.target))
    ) {
      dropdownMenu.classList.toggle("active");
    } else if (dropdownMenu && !dropdownMenu.contains(e.target)) {
      dropdownMenu.classList.remove("active");
    }
  });

  // Close menu when clicking on a menu item
  document.querySelectorAll(".dropdown-item").forEach((item) => {
    item.addEventListener("click", () => {
      const dropdownMenu = document.getElementById("dropdownMenu");
      if (dropdownMenu) {
        dropdownMenu.classList.remove("active");
      }
    });
  });
});

// Todo menu handlers
document.addEventListener("click", async function (e) {
  // Handle menu button clicks or clicks on due date/recurrence info
  const menuBtn = e.target.closest(".todo-menu-btn");
  const dueDateEl = e.target.closest(".due-date, .recurrence-info");

  // Don't do anything if clicking on time or date inputs
  if (
    e.target.closest(
      ".todo-time-input, .todo-date-input, .recurrence-count, .recurrence-unit",
    )
  ) {
    e.stopPropagation();
    return;
  }

  // If clicking on due date or recurrence info, find the menu button
  if (dueDateEl && !menuBtn) {
    const li = dueDateEl.closest("li.todo-item");
    if (li) {
      const btn = li.querySelector(".todo-menu-btn");
      if (btn) btn.click();
      return;
    }
  }

  if (menuBtn) {
    e.stopPropagation();
    e.preventDefault();
    const li = menuBtn.parentElement;
    const menu = li.querySelector(".todo-menu");

    // Close all other open menus
    document.querySelectorAll(".todo-menu").forEach((m) => {
      if (m !== menu) m.style.display = "none";
    });

    // Toggle current menu
    menu.style.display = menu.style.display === "block" ? "none" : "block";

    // Close menu when clicking outside
    if (menu.style.display === "block") {
      const closeMenu = function (e) {
        // Don't close if clicking on menu or its inputs
        if (
          !menu.contains(e.target) &&
          e.target !== menuBtn &&
          !e.target.closest(
            ".todo-time-input, .todo-date-input, .recurrence-count, .recurrence-unit",
          )
        ) {
          menu.style.display = "none";
          document.removeEventListener("click", closeMenu);
        }
      };
      setTimeout(() => {
        document.addEventListener("click", closeMenu);
      }, 0);
    }
    return;
  }

  // Handle menu item clicks
  const menuItem = e.target.closest(".todo-menu-item");
  if (menuItem) {
    e.preventDefault();
    e.stopPropagation();

    const action = menuItem.dataset.action;
    const li = menuItem.closest(".todo-item");
    if (!li) return;

    if (action === "delete") {
      const todoId = menuItem.dataset.id;
      const confirmed = confirm("Are you sure you want to delete this todo?");
      if (!confirmed) return;

      try {
        await deleteTodo(todoId);
      } catch (err) {
        console.error("Error deleting todo:", err);
      }
    } else if (action === "save") {
      const todoId = menuItem.dataset.id;
      const checkbox = li.querySelector(".todo-checkbox");
      const titleEl = li.querySelector(".todo-text");
      const dateInput = li.querySelector(".todo-date-input");
      const timeInput = li.querySelector(".todo-time-input");
      const countEl = li.querySelector(".recurrence-count");
      const unitEl = li.querySelector(".recurrence-unit");

      // Combine date and time inputs using strict RFC3339 formatting
      let dueDate = null;
      if (dateInput && dateInput.value === "") {
        dueDate = "";
      } else if (timeInput?.value && (!dateInput || !dateInput.value)) {
        const now = new Date();
        const [hours, minutes] = timeInput.value.split(":").map(Number);
        const localDate = new Date(
          now.getFullYear(),
          now.getMonth(),
          now.getDate(),
          hours,
          minutes,
          0,
          0,
        );
        dueDate = toRFC3339NoMillis(localDate);
      } else if (dateInput?.value) {
        const dateParts = dateInput.value.split("-");
        const localDate = new Date(
          parseInt(dateParts[0]),
          parseInt(dateParts[1]) - 1,
          parseInt(dateParts[2]),
        );
        if (timeInput?.value) {
          const [hours, minutes] = timeInput.value.split(":").map(Number);
          localDate.setHours(hours, minutes, 0, 0);
        } else {
          localDate.setHours(0, 0, 0, 0);
        }
        dueDate = toRFC3339NoMillis(localDate);
      } else if (dateInput?.dataset.due && !dateInput.value) {
        dueDate = dateInput.dataset.due;
        if (dueDate && !dueDate.endsWith("Z")) {
          const date = new Date(dueDate);
          if (!isNaN(date.getTime())) {
            dueDate = toRFC3339NoMillis(date);
          }
        }
      }
      // Log outgoing payload and due_date value/type
      const outgoingPayload = {
        id: Number(todoId),
        title: titleEl ? titleEl.textContent : "",
        completed: checkbox ? checkbox.checked : false,
        due_date: dueDate,
        recurrence_interval:
          countEl && countEl.value ? Number(countEl.value) : null,
        recurrence_unit: unitEl && unitEl.value ? unitEl.value : null,
        position: Number(li.dataset.position),
      };

      try {
        const response = await fetch("/api/todo", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(outgoingPayload),
        });
        if (!response.ok) throw new Error("Failed to update todo");
        // Close menu after successful save
        const menu = li.querySelector(".todo-menu");
        if (menu) menu.style.display = "none";
        await loadTodosByProject();
      } catch (err) {
        console.error("Error updating todo:", err);
        alert("Failed to update todo. Please try again.");
      }
    } else if (action === "cancel") {
      // Just close the menu without saving
      const menu = li.querySelector(".todo-menu");
      if (menu) menu.style.display = "none";
      await loadTodosByProject();
    }
    return;
  }

  // Close all menus when clicking outside
  document.querySelectorAll(".todo-menu").forEach((m) => {
    m.style.display = "none";
  });
});

// Handle keydown events on form inputs
document.addEventListener("keydown", function (e) {
  // Check if Enter key is pressed on date, time, or recurrence inputs
  if (
    e.key === "Enter" &&
    e.target.matches(
      ".todo-date-input, .todo-time-input, .recurrence-count, .recurrence-unit",
    )
  ) {
    e.preventDefault();
    const li = e.target.closest(".todo-item");
    if (li) {
      const saveBtn = li.querySelector('.todo-menu-item[data-action="save"]');
      if (saveBtn) {
        saveBtn.click();
      }
    }
  }
});

// Handle date and time input changes
document.addEventListener("change", function (e) {
  const input = e.target;
  const li = input.closest(".todo-item");
  if (!li) return;

  // When date input is cleared, clear time and recurrence inputs
  if (input.classList.contains("todo-date-input")) {
    if (!input.value) {
      const timeInput = li.querySelector(".todo-time-input");
      const countEl = li.querySelector(".recurrence-count");
      const unitEl = li.querySelector(".recurrence-unit");
      if (timeInput) timeInput.value = "";
      if (countEl) countEl.value = "";
      if (unitEl) unitEl.value = "day";
    }
  }
  // When time is set but no date, set date to today
  else if (input.classList.contains("todo-time-input") && input.value) {
    const dateInput = li.querySelector(".todo-date-input");
    if (dateInput && !dateInput.value) {
      const today = new Date();
      const yyyy = today.getFullYear();
      const mm = String(today.getMonth() + 1).padStart(2, "0");
      const dd = String(today.getDate()).padStart(2, "0");
      dateInput.value = `${yyyy}-${mm}-${dd}`;
    }
  }
});

// Auto-resize textarea height as the user types
document.addEventListener("input", function (e) {
  const ta = e.target;
  if (ta && ta.tagName === "TEXTAREA") {
    ta.style.height = "auto";
    ta.style.height = ta.scrollHeight + "px";
  }
});

// Drag & Drop handlers
document.addEventListener("dragstart", function (e) {
  if (e.target.closest("textarea")) return;

  // If dragging the menu button, find the parent todo item
  const menuBtn = e.target.closest(".todo-menu-btn");
  const projectTitle = e.target.closest(".project-title");

  if (projectTitle && projectTitle.getAttribute("draggable") === "true") {
    const id = projectTitle.dataset.id;
    e.dataTransfer.setData(
      "application/json",
      JSON.stringify({ type: "project", projectId: id }),
    );
    e.dataTransfer.effectAllowed = "move";
    projectTitle.closest(".project-item").classList.add("dragging");
    return; // handled project drag
  }

  // Get the todo item, either directly or from the menu button
  const item = menuBtn
    ? menuBtn.closest(".todo-item")
    : e.target.closest(".todo-item");
  if (item) {
    // Prevent dragging todos from the "This Week" project
    if (item.dataset.projectId === "thisweek") {
      e.preventDefault();
      return;
    }
    e.dataTransfer.setData(
      "application/json",
      JSON.stringify({
        id: item.dataset.id,
        fromProject: item.dataset.projectId,
        fromCompleted: item.dataset.completed,
      }),
    );
    e.dataTransfer.effectAllowed = "move";
    item.classList.add("dragging");
  }
});
document.addEventListener("dragend", function (e) {
  if (e.target.closest("textarea")) return;
  const proj = e.target.closest(".project-item");
  if (proj && proj.classList.contains("dragging")) {
    proj.classList.remove("dragging");
  }
  const item = e.target.closest(".todo-item");
  if (item) {
    item.classList.remove("dragging");
  }
});
document.addEventListener("dragover", function (e) {
  if (e.target.closest("textarea")) return; // allow default drag over textarea

  // Check if we're over a todo from "This Week" project
  const todoItem = e.target.closest(".todo-item");
  if (todoItem && todoItem.dataset.projectId === "thisweek") {
    return; // Don't allow dropping on todos from "This Week" project
  }

  // Check if we're over the "This Week" project
  const projectItem = e.target.closest(".project-item");
  if (projectItem) {
    const projectTitle = projectItem.querySelector(".project-title");
    if (projectTitle && projectTitle.dataset.id === "thisweek") {
      return; // Don't allow dropping on the "This Week" project
    }
  }

  // Allow dropping on other lists and items
  if (
    e.target.closest(".todo-list") ||
    e.target.closest(".todo-item") ||
    e.target.closest(".project-item")
  ) {
    e.preventDefault();
  }
});
document.addEventListener("drop", async function (e) {
  if (e.target.closest("textarea")) return;
  let payload = null;
  try {
    payload = JSON.parse(e.dataTransfer.getData("application/json") || "{}");
  } catch {}

  // PROJECT REORDER
  if (payload && payload.type === "project") {
    const projectsContainer = document.querySelector(".projects-container");
    const targetProjectItem = e.target.closest(".project-item");
    if (!targetProjectItem) return;

    const draggedEl = projectsContainer
      .querySelector(`.project-title[data-id="${payload.projectId}"]`)
      ?.closest(".project-item");
    if (!draggedEl || draggedEl === targetProjectItem) return;

    // Prevent dropping to the left of the "This Week" project
    const thisWeekProject = projectsContainer.querySelector(
      '.project-title[data-id="thisweek"]',
    );
    if (thisWeekProject) {
      const thisWeekRect = thisWeekProject.getBoundingClientRect();
      if (e.clientX <= thisWeekRect.right) {
        return; // Don't allow dropping to the left of "This Week"
      }
    }

    const rect = targetProjectItem.getBoundingClientRect();
    if (e.clientX > rect.left + rect.width / 2) {
      projectsContainer.insertBefore(draggedEl, targetProjectItem.nextSibling);
    } else {
      projectsContainer.insertBefore(draggedEl, targetProjectItem);
    }

    // Build new order excluding special/non-numeric projects
    const ids = Array.from(projectsContainer.querySelectorAll(".project-title"))
      .map((el) => el.dataset.id)
      .filter((id) => !isNaN(parseInt(id)))
      .map((id) => parseInt(id));
    try {
      await fetch("/api/projects/reorder", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(ids),
      });
    } catch (err) {
      console.error("Error reordering projects:", err);
    }
    return; // prevent todo handling
  }
  const dropTargetItem = e.target.closest(".todo-item");
  let list = e.target.closest(".todo-list");
  if (!list && dropTargetItem) {
    list = dropTargetItem.parentElement.closest(".todo-list");
  }
  if (!list) return;
  e.preventDefault();
  const dataStr = e.dataTransfer.getData("application/json");
  if (!dataStr) return;
  let data;
  try {
    data = JSON.parse(dataStr);
  } catch (err) {
    return;
  }

  // Prevent moving todos from the "This Week" project
  if (data.fromProject === "thisweek") {
    return;
  }

  const todoId = Number(data.id);
  const targetProject = Number(list.dataset.projectId);
  const targetCompleted = list.dataset.completed === "1";
  const sameList =
    Number(data.fromProject) === targetProject &&
    data.fromCompleted === (targetCompleted ? "1" : "0");
  if (Number.isNaN(todoId) || Number.isNaN(targetProject)) return;
  try {
    // gather current todo details so we don't lose them when moving
    // Only move the todo in the destination list (not in special lists)
    let itemEl = null;
    itemEl = Array.from(document.querySelectorAll(".todo-item")).find(
      (el) =>
        el.dataset.projectId === data.fromProject &&
        Number(el.dataset.id) === todoId,
    );

    // Fallback for legacy cases
    if (!itemEl) {
      itemEl = document.querySelector(`.todo-item[data-id="${todoId}"]`);
    }
    const titleEl = itemEl ? itemEl.querySelector(".todo-text") : null;
    const title = titleEl ? titleEl.textContent : "";
    const dateInput = itemEl ? itemEl.querySelector(".todo-date-input") : null;
    const timeInput = itemEl ? itemEl.querySelector(".todo-time-input") : null;
    let dueDate = null;
    if (dateInput?.value) {
      // Create a date object from the input values
      const [year, month, day] = dateInput.value.split("-").map(Number);
      let date = new Date();
      date.setFullYear(year, month - 1, day);

      // Set time if provided
      if (timeInput?.value) {
        const [hours, minutes] = timeInput.value.split(":").map(Number);
        date.setHours(hours, minutes, 0, 0);
      } else {
        date.setHours(0, 0, 0, 0);
      }

      dueDate = toRFC3339NoMillis(date);
    } else if (dateInput?.dataset.due) {
      // If we have a due date from dataset, ensure it's in the correct format
      const date = new Date(dateInput.dataset.due);
      if (!isNaN(date.getTime())) {
        dueDate = toRFC3339NoMillis(date);
      } else {
        dueDate = dateInput.dataset.due;
      }
    }
    const countEl = itemEl ? itemEl.querySelector(".recurrence-count") : null;
    const unitEl = itemEl ? itemEl.querySelector(".recurrence-unit") : null;
    const recInt = countEl && countEl.value ? Number(countEl.value) : null;
    const recUnitVal = unitEl && unitEl.value ? unitEl.value : null;

    const response = await fetch("/api/todo", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: todoId,
        title: title,
        completed: targetCompleted,
        project_id: targetProject,
        due_date: dueDate,
        recurrence_interval: recInt,
        recurrence_unit: recUnitVal,
      }),
    });
    if (!response.ok) {
      throw new Error("Failed to move todo");
    }
    const draggedEl = itemEl;
    if (draggedEl) {
      if (dropTargetItem && dropTargetItem !== draggedEl) {
        const rect = dropTargetItem.getBoundingClientRect();
        const before = e.clientY < rect.top + rect.height / 2;
        if (before) {
          dropTargetItem.parentElement.insertBefore(draggedEl, dropTargetItem);
        } else {
          dropTargetItem.parentElement.insertBefore(
            draggedEl,
            dropTargetItem.nextSibling,
          );
        }
      } else if (!dropTargetItem) {
        // Dropped on empty space within the list – move to end
        list.appendChild(draggedEl);
      } else {
        // dropped on itself – do nothing
        return;
      }
    }

    // collect IDs after DOM move and update each element's data-position
    const ids = Array.from(list.querySelectorAll(".todo-item")).map(
      (el, idx) => {
        el.dataset.position = idx + 1; // keep dataset in sync with visual order
        return Number(el.dataset.id);
      },
    );

    // Persist the new order in the backend
    const reorderPromise = fetch("/api/todos/reorder", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(ids),
    });

    // Always wait for the backend to finish updating positions
    await reorderPromise;

    if (!sameList) {
      // Moving between lists or projects – reload full UI to refresh counts etc.
      await loadTodosByProject();
    }
  } catch (err) {
    console.error("Error moving todo:", err);
    alert("Failed to move todo. Please try again.");
  }
});

async function addProject() {
  try {
    const response = await fetch("/api/projects", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ title: "New Project" }),
    });

    if (!response.ok) {
      throw new Error("Failed to create project");
    }

    // Get the created project
    const project = await response.json();

    // Close the dropdown menu
    const dropdownMenu = document.getElementById("dropdownMenu");
    if (dropdownMenu) {
      dropdownMenu.classList.remove("active");
    }

    // Load todos with the new project
    await loadTodosByProject();

    // Scroll the new project into view
    setTimeout(() => {
      const projectEl = document.querySelector(
        `.project-title[data-id="${project.id}"]`,
      );
      if (projectEl) {
        projectEl
          .closest(".project-item")
          ?.scrollIntoView({
            behavior: "smooth",
            block: "nearest",
            inline: "nearest",
          });
      }
    }, 50);
  } catch (error) {
    console.error("Error adding project:", error);
    alert("Failed to add project. Please try again.");
  }
}

async function importGoogleTasks(exportJson) {
  if (!exportJson) throw new Error("No data to import");

  // Create and show modal
  const modal = document.createElement("div");
  modal.className = "import-modal";
  modal.innerHTML = `
                <div class="import-modal-content">
                    <div class="import-modal-header">
                        <h3 class="import-modal-title">Importing Google Tasks</h3>
                        <button class="import-close-btn" disabled>&times;</button>
                    </div>
                    
                    <div class="progress-section">
                        <h4>Projects</h4>
                        <div class="progress-bar-container">
                            <div class="progress-bar" id="project-progress"></div>
                        </div>
                        <div class="progress-stats">
                            <span id="project-stats">0/0 projects</span>
                            <span id="project-status">Starting import...</span>
                        </div>
                        <div class="progress-details" id="project-details"></div>
                    </div>
                    

                </div>
            `;
  document.body.appendChild(modal);

  // Show modal with animation
  setTimeout(() => modal.classList.add("visible"), 10);

  // Helper function to update progress
  const updateProgress = (
    current,
    total,
    status,
    isComplete = false,
    isError = false,
  ) => {
    const progressBar = document.getElementById("project-progress");
    const statsEl = document.getElementById("project-stats");
    const statusEl = document.getElementById("project-status");

    if (progressBar) {
      const progress = total > 0 ? Math.round((current / total) * 100) : 0;
      progressBar.style.width = `${progress}%`;

      if (isComplete) {
        progressBar.classList.add("complete");
        statusEl.textContent = "Completed";
      } else if (isError) {
        progressBar.classList.add("error");
      }
    }

    if (statsEl) {
      statsEl.textContent = `${current}/${total} project${total !== 1 ? "s" : ""}`;
    }

    if (statusEl && status) {
      statusEl.textContent = status;
    }
  };

  // Helper function to log errors
  const logError = (error) => {
    console.error(error);
    const detailsEl = document.getElementById("project-details");
    if (!detailsEl) return;

    const entry = document.createElement("div");
    entry.className = "progress-item error";
    entry.innerHTML = `
                    <span class="progress-item-icon">✗</span>
                    <span class="progress-item-text">${error}</span>
                `;
    detailsEl.appendChild(entry);
    detailsEl.scrollTop = detailsEl.scrollHeight;
  };

  try {
    // Handle Google Tasks API export format
    const taskLists = exportJson.items || [];
    if (taskLists.length === 0) {
      throw new Error("No task lists found in the export");
    }

    let totalTasks = 0;
    let importedTasks = 0;
    let importedProjects = 0;

    // First count all tasks and create project entries
    for (const list of taskLists) {
      const tasks = (list.items || []).filter((t) => t.title);
      totalTasks += tasks.length;

      if (tasks.length > 0) {
        const projectEntry = document.createElement("div");
        projectEntry.className = "progress-item progress-item-pending";
        projectEntry.innerHTML = `
                            <span class="progress-item-icon">⏳</span>
                            <span class="progress-item-text">${list.title || "Untitled Project"}</span>
                            <span class="progress-item-stats">0/${tasks.length} tasks</span>
                        `;
        document.getElementById("project-details").appendChild(projectEntry);
      }
    }

    if (totalTasks === 0) {
      throw new Error("No tasks found in the export");
    }

    // Update project progress
    updateProgress(0, taskLists.length, "Starting project import...");

    // Process each project
    for (let i = 0; i < taskLists.length; i++) {
      const list = taskLists[i];
      const title = list.title || "Untitled Project";
      const tasks = (list.items || []).filter((t) => t.title);

      if (tasks.length === 0) continue;

      // Update project status
      const projectEntries = document.querySelectorAll(
        "#project-details .progress-item",
      );
      const currentProjectEl = projectEntries[importedProjects];

      if (currentProjectEl) {
        currentProjectEl.innerHTML = `
                            <span class="progress-item-icon">🔄</span>
                            <span class="progress-item-text">${title}</span>
                            <span class="progress-item-stats">0/${tasks.length} tasks</span>
                        `;
      }
      currentProjectEl.scrollIntoView({ behavior: "smooth" });

      updateProgress(
        importedProjects,
        taskLists.length,
        `Importing project: ${title}...`,
      );

      try {
        // Create project
        const createResp = await fetch("/api/projects", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ title }),
        });

        if (!createResp.ok) {
          throw new Error(
            `Failed to create project: ${await createResp.text()}`,
          );
        }

        const project = await createResp.json();

        // Update project entry
        if (currentProjectEl) {
          currentProjectEl.innerHTML = `
                                <span class="progress-item-icon">✓</span>
                                <span class="progress-item-text">${title}</span>
                                <span class="progress-item-stats">0/${tasks.length} tasks</span>
                            `;
        }

        // Process tasks for this project
        for (let j = 0; j < tasks.length; j++) {
          const task = tasks[j];

          try {
            const todo = {
              title: task.title || "",
              completed: task.status === "completed" || !!task.completed,
              project_id: project.id,
              due_date: task.due
                ? new Date(task.due).toISOString().split("T")[0]
                : null,
            };

            const taskResp = await fetch("/api/todo", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(todo),
            });

            if (!taskResp.ok) {
              throw new Error(
                `Failed to import task: ${await taskResp.text()}`,
              );
            }

            importedTasks++;

            // Update project task count in the UI
            if (currentProjectEl) {
              const taskCount = currentProjectEl.querySelector(
                ".progress-item-stats",
              );
              if (taskCount) {
                taskCount.textContent = `(${j + 1}/${tasks.length} tasks)`;
              }

              // Update the main status with current progress
              updateProgress(
                importedProjects,
                taskLists.length,
                `Importing ${title} (${j + 1}/${tasks.length} tasks)...`,
              );
            }
          } catch (taskError) {
            logError(
              `Error importing task: ${task.title} - ${taskError.message}`,
            );
          }
        }

        importedProjects++;
        updateProgress(
          importedProjects,
          taskLists.length,
          "Project import in progress...",
        );
      } catch (projectError) {
        console.error(`Error importing project ${title}:`, projectError);

        if (currentProjectEl) {
          currentProjectEl.innerHTML = `
                                <span class="progress-item-icon">✗</span>
                                <span class="progress-item-text">${title} - ${projectError.message}</span>
                            `;
          currentProjectEl.classList.add("error");
        }

        // Skip to next project
        importedProjects++;
        continue;
      }
    }

    // Update final status
    updateProgress(
      importedProjects,
      taskLists.length,
      "All projects imported",
      true,
    );

    // Enable close button
    const closeBtn = modal.querySelector(".import-close-btn");
    if (closeBtn) {
      closeBtn.disabled = false;
      closeBtn.onclick = () => {
        modal.classList.remove("visible");
        setTimeout(() => {
          document.body.removeChild(modal);
          loadTodosByProject(); // Refresh the UI
        }, 300);
      };
    }
  } catch (error) {
    console.error("Import failed:", error);

    // Update UI with error
    const statusEl = document.getElementById("project-status");
    if (statusEl) {
      statusEl.textContent = `Error: ${error.message}`;
      statusEl.style.color = "#f44336";
    }

    // Enable close button on error
    const closeBtn = modal.querySelector(".import-close-btn");
    if (closeBtn) {
      closeBtn.disabled = false;
      closeBtn.onclick = () => {
        modal.classList.remove("visible");
        setTimeout(() => {
          document.body.removeChild(modal);
        }, 300);
      };
    }

    throw error; // Re-throw to be caught by the caller
  }
}

async function loadTodosByProject() {
  try {
    // Fetch all projects first
    const projectsResp = await fetch("/api/projects");
    if (!projectsResp.ok) {
      throw new Error("Failed to fetch projects");
    }
    const projectsList = await projectsResp.json();

    // If no projects, clear the UI and return early
    if (!projectsList || projectsList.length === 0) {
      document.querySelector(".projects-container").innerHTML =
        "<p>No projects yet. Create one to get started!</p>";
      return;
    }

    // Fetch all todos
    const todosResponse = await fetch("/api/todos");
    if (!todosResponse.ok) {
      throw new Error("Failed to fetch todos");
    }
    const todos = await todosResponse.json();

    // Get projects
    const projectsResponse = await fetch("/api/projects");
    const projects = await projectsResponse.json();

    const projectsContainer = document.querySelector(".projects-container");
    projectsContainer.innerHTML = "";

    // Add special "This Week" project section for upcoming todos
    const today = new Date();
    const thisWeek = new Date();
    thisWeek.setDate(today.getDate() + 6);
    const upcomingTodos = todos.filter((todo) => {
      if (!todo.due_date) return false;
      const due = new Date(todo.due_date);
      return due <= thisWeek;
    });

    // Sort by due_date in ascending order
    upcomingTodos.sort((a, b) => {
      if (a.due_date && b.due_date) {
        return new Date(a.due_date) - new Date(b.due_date);
      }
      return 0;
    });

    if (upcomingTodos.length > 0) {
      const upcomingGroup = await loadUpcomingTodos(upcomingTodos);
      projectsContainer.appendChild(upcomingGroup);
    }

    // Create a project group for each project
    for (const project of projects) {
      const projectTodos = todos.filter(
        (todo) => todo.project_id === project.id,
      );

      // Sort todos by their position field ascending so both active and completed lists respect persisted order
      projectTodos.sort((a, b) => (a.position || 0) - (b.position || 0));

      const projectGroup = await loadProject(project, projectTodos);
      projectsContainer.appendChild(projectGroup);
    }
  } catch (error) {
    console.error("Error loading todos by project:", error);
    alert("Failed to load todos. Please try again.");
  }
}

async function loadProject(project, filteredTodos) {
  try {
    const projectGroup = document.createElement("div");
    projectGroup.className = "project-item";
    projectGroup.innerHTML = `
                    <div class="project-title-container">
                        <div class="project-title" data-id="${project.id}" ${project.id !== "thisweek" ? 'draggable="true"' : ""}>${project.title}</div>
                        <div class="active-count-badge"></div>
                        <button class="delete-project-btn" data-id="${project.id}">✕</button>
                    </div>
                    <div class="todo-form">
                        <textarea class="todo-input" data-id="${project.id}" rows="1" placeholder="Add a new todo..."></textarea>
                    </div>
                `;

    const activeList = document.createElement("div");
    activeList.className = "active-todos";
    activeList.innerHTML = `
                    <ul class="todo-list" data-project-id="${project.id}" data-completed="0"></ul>
                `;

    const completedList = document.createElement("div");
    completedList.className = "completed-todos";
    completedList.innerHTML = `
                    <button class="toggle-completed-btn" data-id="${project.id}">
                        <span class="arrow">▼</span>
                        <span class="toggle-text">Completed</span>
                        <span class="completed-count-badge"></span>
                    </button>
                    <ul class="todo-list" data-project-id="${project.id}" data-completed="1"></ul>
                `;

    const activeTodos = activeList.querySelector(".todo-list");
    const completedTodos = completedList.querySelector(".todo-list");

    filteredTodos.forEach((todo) => {
      const li = document.createElement("li");
      li.className = `todo-item ${todo.completed ? "completed" : ""}`;
      li.setAttribute("draggable", "true");
      li.dataset.id = todo.id;
      li.dataset.projectId = project.id;
      li.dataset.completed = todo.completed ? "1" : "0";
      li.dataset.position = todo.position;
      let dueDateHtml = "";
      let recurrenceHtml = "";
      if (todo.due_date) {
        // Parse RFC3339 UTC string and convert to local time
        const utcDate = new Date(todo.due_date);
        const pad = (n) => String(n).padStart(2, "0");
        const localYear = utcDate.getFullYear();
        const localMonth = pad(utcDate.getMonth() + 1);
        const localDay = pad(utcDate.getDate());
        const localHours = pad(utcDate.getHours());
        const localMinutes = pad(utcDate.getMinutes());
        const datePart = `${localYear}-${localMonth}-${localDay}`;
        const timeStr = `${localHours}:${localMinutes}`;

        // Build today's local date string for comparison
        const now = new Date();
        const todayLocal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;

        let dueDateClass = "";
        if (datePart === todayLocal) {
          dueDateClass = "today";
        } else if (datePart < todayLocal && !todo.completed) {
          dueDateClass = "overdue";
        }
        dueDateHtml = `<div class="due-date ${dueDateClass}"><i class="nf nf-md-calendar"></i> ${datePart} <i class="nf nf-fa-clock"></i> ${timeStr}</div>`;
        if (todo.recurrence_interval && todo.recurrence_unit) {
          recurrenceHtml = `<div class="recurrence-info ${dueDateClass}"><i class="nf nf-md-refresh"></i> Every ${todo.recurrence_interval} ${todo.recurrence_unit}(s)</div>`;
        }
      }

      li.innerHTML = `
                            <input class="todo-checkbox" type="checkbox" onchange="toggleTodo(${todo.id})" ${todo.completed ? "checked" : ""}>
                            <div class="todo-content">
                                <pre class="todo-text" data-id="${todo.id}">${linkify(hashtagify(todo.title))}</pre>
                                ${dueDateHtml}
                                ${recurrenceHtml}
                            </div>
                            <button class="todo-menu-btn" data-id="${todo.id}">⋮</button>
                            <div class="todo-menu">
                                <div class="todo-menu-item" style="display:flex; flex-direction:column; gap:4px;">
                                    <div style="display:flex; align-items:center; gap:8px;">
                                        <span>Date</span>
                                        <input type="date" class="todo-date-input" data-id="${todo.id}" data-due="${todo.due_date || ""}" value="${(() => {
                                          if (!todo.due_date) return "";
                                          const d = new Date(todo.due_date);
                                          return (
                                            d.getFullYear() +
                                            "-" +
                                            String(d.getMonth() + 1).padStart(
                                              2,
                                              "0",
                                            ) +
                                            "-" +
                                            String(d.getDate()).padStart(2, "0")
                                          );
                                        })()}">
                                    </div>
                                    <div style="display:flex; align-items:center; gap:8px;">
                                        <span>Time</span>
                                        <input type="time" class="todo-time-input" data-id="${todo.id}" value="${(() => {
                                          if (!todo.due_date) return "";
                                          const d = new Date(todo.due_date);
                                          return (
                                            String(d.getHours()).padStart(
                                              2,
                                              "0",
                                            ) +
                                            ":" +
                                            String(d.getMinutes()).padStart(
                                              2,
                                              "0",
                                            )
                                          );
                                        })()}">
                                    </div>
                                </div>
                                <div class="todo-menu-item" style="display:flex; align-items:center; gap:8px;">
                                    <span>Repeat</span>
                                    <input type="number" min="0" class="recurrence-count" data-id="${todo.id}" value="${todo.recurrence_interval || ""}" style="width:60px;">
                                    <select class="recurrence-unit" data-id="${todo.id}">
                                        <option value="day" ${todo.recurrence_unit === "day" ? "selected" : ""}>day(s)</option>
                                        <option value="week" ${todo.recurrence_unit === "week" ? "selected" : ""}>week(s)</option>
                                        <option value="month" ${todo.recurrence_unit === "month" ? "selected" : ""}>month(s)</option>
                                        <option value="year" ${todo.recurrence_unit === "year" ? "selected" : ""}>year(s)</option>
                                    </select>
                                </div>
                                <div class="todo-menu-item" data-action="save" data-id="${todo.id}">Save</div>
                                <div class="todo-menu-item" data-action="cancel">Cancel</div>
                                <div class="todo-menu-item" data-action="delete" data-id="${todo.id}">Delete</div>
                            </div>
                        `;

      if (todo.completed) {
        completedTodos.appendChild(li);
      } else {
        activeTodos.appendChild(li);
      }
    });

    const projectTitleContainer = projectGroup.querySelector(
      ".project-title-container",
    );
    const activeCountBadge = projectTitleContainer.querySelector(
      ".active-count-badge",
    );
    activeCountBadge.textContent = `(${filteredTodos.filter((t) => !t.completed).length})`;

    // Update toggle button count
    const toggleBtn = completedList.querySelector(".toggle-completed-btn");
    const completedCountBadge = toggleBtn.querySelector(
      ".completed-count-badge",
    );
    completedCountBadge.textContent = `(${filteredTodos.filter((t) => t.completed).length})`;

    projectGroup.appendChild(activeList);
    projectGroup.appendChild(completedList);

    // Add click handler for project titles
    projectGroup
      .querySelector(".project-title")
      .addEventListener("click", function (e) {
        if (!editingProject) {
          startProjectEdit(e.target);
        }
      });

    const todoInput = projectGroup.querySelector(".todo-input");

    // Add key handlers for todo input
    todoInput.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        // Clear the input on Escape
        this.value = "";
        autoResizeTextarea(this);
      } else if (e.key === "Enter" && !e.shiftKey) {
        // Submit on Enter (without Shift)
        e.preventDefault();
        addTodo(this);
      }
    });

    // Setup auto-resize for the todo input
    setupAutoResize(todoInput);

    activeTodos.querySelectorAll(".todo-text").forEach((list) => {
      list.addEventListener("click", function (e) {
        if (e.target.closest("a")) return; // Allow link clicks
        const id = this.dataset.id;
        editTodo(id, this);
      });
    });

    completedTodos.querySelectorAll(".todo-text").forEach((list) => {
      list.addEventListener("click", function (e) {
        if (e.target.closest("a")) return; // Allow link clicks
        const id = this.dataset.id;
        editTodo(id, this);
      });
    });

    if (localStorage.getItem("completedExpanded_" + project.id) !== "true") {
      const arrow = toggleBtn.querySelector(".arrow");
      completedTodos.style.display = "none";
      arrow.textContent = "▶";
    }

    completedList
      .querySelector(".toggle-completed-btn")
      .addEventListener("click", function () {
        const list = this.nextElementSibling;
        const arrow = this.querySelector(".arrow");

        if (
          localStorage.getItem("completedExpanded_" + project.id) !== "true"
        ) {
          list.style.display = "block";
          arrow.textContent = "▼";
          localStorage.setItem("completedExpanded_" + project.id, "true");
        } else {
          list.style.display = "none";
          arrow.textContent = "▶";
          localStorage.setItem("completedExpanded_" + project.id, "false");
        }
      });

    return projectGroup;
  } catch (error) {
    console.error("Error loading todos:", error);
    alert("Failed to load todos. Please try again.");
  }
}

// Export database handler
document
  .getElementById("exportDatabase")
  .addEventListener("click", async function () {
    try {
      // Get all projects, todos, and subscriptions in parallel
      const [todosRes, projectsRes, subscriptionsRes] = await Promise.all([
        fetch("/api/todos"),
        fetch("/api/projects"),
        fetch("/api/ics_subscriptions"),
      ]);

      if (!todosRes.ok) throw new Error("Failed to fetch todos");
      if (!projectsRes.ok) throw new Error("Failed to fetch projects");
      if (!subscriptionsRes.ok)
        throw new Error("Failed to fetch subscriptions");

      let todos = await todosRes.json();
      let projects = await projectsRes.json();
      let subscriptions = await subscriptionsRes.json();

      // Filter out the special 'thisweek' project and its todos
      projects = projects.filter((p) => p.id !== "thisweek");
      const projectIds = new Set(projects.map((p) => p.id));
      todos = todos.filter((todo) => projectIds.has(todo.project_id));

      // Combine data
      const exportData = {
        version: 1,
        exportedAt: new Date().toISOString(),
        projects: projects,
        todos: todos,
        subscriptions: subscriptions,
      };

      // Create a blob and download link
      const blob = new Blob([JSON.stringify(exportData, null, 2)], {
        type: "application/json",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `todo-export-${new Date().toISOString()}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error("Export failed:", error);
      alert("Failed to export database. Check console for details.");
    }
  });

// Import database handler
document
  .getElementById("importDatabase")
  .addEventListener("click", function () {
    document.getElementById("importDbFile").click();
  });

// Handle database import file selection
document
  .getElementById("importDbFile")
  .addEventListener("change", async function (e) {
    const file = e.target.files[0];
    if (!file) return;

    try {
      const data = JSON.parse(await file.text());

      // Validate the import data
      if (!data.version || !data.projects || !data.todos) {
        throw new Error("Invalid import file format");
      }

      // Confirm with user before proceeding with import
      if (
        !confirm(
          `WARNING: This will DELETE ALL existing projects and todos, then import ${data.projects.length} projects and ${data.todos.length} todos. This action cannot be undone. Continue?`,
        )
      ) {
        return;
      }

      // Delete all existing todos and projects
      try {
        const projects = await fetch("/api/projects").then((res) =>
          res.ok ? res.json() : [],
        );
        for (const project of projects) {
          await fetch(`/api/projects/${project.id}`, {
            method: "DELETE",
          });
        }
        const subscriptions = await fetch("/api/ics_subscriptions").then(
          (res) => (res.ok ? res.json() : []),
        );
        for (const sub of subscriptions) {
          await fetch(`/api/cancel_ics_subscription?id=${sub.id}`, {
            method: "DELETE",
          });
        }
      } catch (err) {
        console.error("Failed to clear existing data:", err);
        throw new Error("Failed to clear existing data");
      }

      // Import projects first (excluding any special projects)
      const projectsToImport = data.projects.filter((p) => p.id !== "thisweek");

      for (const project of projectsToImport) {
        try {
          // Create new project with only the necessary fields
          const projectData = {
            title: project.title,
            position: project.position || 0,
          };

          await fetch("/api/projects", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(projectData),
          });
        } catch (err) {
          console.error(`Failed to import project ${project.id}:`, err);
        }
      }

      // Get the newly created projects to map old IDs to new ones
      const newProjects = await fetch("/api/projects").then((res) =>
        res.ok ? res.json() : [],
      );
      const projectIdMap = {};

      // Create a mapping from old project IDs to new ones
      for (const oldProject of projectsToImport) {
        const newProject = newProjects.find(
          (p) =>
            p.title === oldProject.title &&
            p.position === (oldProject.position || 0),
        );
        if (newProject) {
          projectIdMap[oldProject.id] = newProject.id;
        }
      }

      // Then import todos (only those that belong to imported projects)
      const todosToImport = data.todos.filter(
        (todo) => projectIdMap[todo.project_id] !== undefined,
      );

      for (const todo of todosToImport) {
        try {
          // Prepare todo data with all required fields
          const todoData = {
            title: todo.title || "",
            completed: !!todo.completed,
            project_id: projectIdMap[todo.project_id] || 1, // Use the mapped project ID
            due_date: todo.due_date || null,
            recurrence_interval: todo.recurrence_interval || null,
            recurrence_unit: todo.recurrence_unit || null,
            position: todo.position || 0,
          };

          // Create new todo
          await fetch("/api/todo", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(todoData),
          });
        } catch (err) {
          console.error(`Failed to import todo ${todo.id}:`, err);
        }
      }

      // Import subscriptions
      if (data.subscriptions) {
        for (const sub of data.subscriptions) {
          try {
            await fetch("/api/subscribe_ics", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                url: sub.url,
                project_name: sub.project_name,
              }),
            });
          } catch (err) {
            console.error(`Failed to import subscription ${sub.url}:`, err);
          }
        }
      }

      alert("Import completed successfully! Refreshing the page...");
      window.location.reload();
    } catch (error) {
      console.error("Import failed:", error);
      alert("Failed to import database. Check console for details.");
    } finally {
      // Reset the file input
      e.target.value = "";
    }
  });

// Import Google Tasks handler
document
  .getElementById("importFile")
  .addEventListener("change", async function (e) {
    const file = e.target.files[0];
    if (!file) return;
    try {
      const text = await file.text();
      const data = JSON.parse(text);

      await importGoogleTasks(data);
      alert("Import completed");
      await loadTodosByProject();
    } catch (err) {
      console.error("Import error:", err, "Stack:", err.stack);
      alert("Import failed: " + (err.message || "Unknown error"));
    } finally {
      e.target.value = "";
    }
  });

// Import Calendar (.ics) handler
document
  .getElementById("importCalendarFile")
  .addEventListener("change", async function (e) {
    const file = e.target.files[0];
    if (!file) return;
    try {
      const text = await file.text();
      const events = parseICS(text);

      if (events.length === 0) {
        alert("No upcoming events found in the calendar file.");
        return;
      }

      const nowUTC = new Date();
      function getNextOccurrence(start, interval, unit) {
        if (!start) return null;
        const next = new Date(start.getTime());
        const inc = interval || 1;
        const step = unit;
        const add = {
          day: () => next.setUTCDate(next.getUTCDate() + inc),
          week: () => next.setUTCDate(next.getUTCDate() + inc * 7),
          month: () => next.setUTCMonth(next.getUTCMonth() + inc),
          year: () => next.setUTCFullYear(next.getUTCFullYear() + inc),
        }[step];
        if (!add) return next; // unsupported
        while (next < nowUTC) {
          add();
        }
        return next;
      }

      // Create a new project named after the uploaded file
      const projectTitle =
        file.name.replace(/\.(ics|calendar)$/i, "") || "Imported Calendar";
      let projectId = 1;
      try {
        const projResp = await fetch("/api/projects", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ title: projectTitle }),
        });
        if (!projResp.ok) throw new Error("Failed to create project");
        const projData = await projResp.json();
        projectId = projData.id;
      } catch (projErr) {
        console.error(
          "Project creation failed, falling back to default project",
          projErr,
        );
      }

      for (const ev of events) {
        // Build payload for each event as a todo
        const todoPayload = {
          title: ev.summary || "Untitled event",
          completed: false,
          project_id: projectId,
          // Determine correct due date
          due_date: (() => {
            if (!ev.date) return null;
            const nextDate = ev.recurrenceUnit
              ? getNextOccurrence(
                  ev.date,
                  ev.recurrenceInterval,
                  ev.recurrenceUnit,
                )
              : ev.date;
            return toRFC3339NoMillis(nextDate);
          })(),
          recurrence_interval: ev.recurrenceInterval,
          recurrence_unit: ev.recurrenceUnit,
        };
        try {
          await fetch("/api/todo", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(todoPayload),
          });
        } catch (err) {
          console.error("Failed to create todo for event", ev, err);
        }
      }

      alert("Calendar import completed");
      await loadTodosByProject();
    } catch (err) {
      console.error("ICS import error:", err);
      alert("Import failed: " + (err.message || "Unknown error"));
    } finally {
      e.target.value = "";
    }
  });

// Auto-resize textarea function
function autoResizeTextarea(textarea) {
  textarea.style.height = "auto";
  textarea.style.height = textarea.scrollHeight + "px";
}

// Add auto-resize event listener to a textarea
function setupAutoResize(textarea) {
  // Initial resize
  autoResizeTextarea(textarea);

  // Resize on input
  textarea.addEventListener("input", function () {
    autoResizeTextarea(this);
  });
}

// Load todos when page loads
document.addEventListener("DOMContentLoaded", async function () {
  // Make menu buttons not draggable to allow drag events to bubble to parent
  document.querySelectorAll(".todo-menu-btn").forEach((btn) => {
    btn.setAttribute("draggable", "false");
  });
  // Check if dark mode is saved in localStorage
  const savedMode = localStorage.getItem("darkMode");
  if (savedMode === "true") {
    document.body.classList.add("dark-mode");
    document.getElementById("darkModeToggle").classList.add("dark-mode");
    localStorage.setItem("darkMode", "true");
  } else {
    document.body.classList.remove("dark-mode");
    document.getElementById("darkModeToggle").classList.remove("dark-mode");
    localStorage.setItem("darkMode", "false");
  }

  try {
    await loadTodosByProject();
  } catch (error) {
    console.error("Error loading projects and todos:", error);
    alert("Failed to load projects and todos. Please try again.");
  }
});

async function addTodo(textarea) {
  if (!textarea.value.trim()) {
    alert("Todo cannot be empty");
    return;
  }

  try {
    const todo = {
      title: textarea.value,
      completed: false,
      project_id: Number(textarea.dataset.id),
    };

    const response = await fetch("/api/todo", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(todo),
    });

    if (!response.ok) {
      const errorData = await response.json();
      throw new Error(`Failed to add todo: ${errorData.message}`);
    }

    textarea.value = "";
    await loadTodosByProject();
  } catch (error) {
    console.error("Error adding todo:", error);
    alert("Failed to add todo. Please try again.");
  }
}

async function editTodo(id, element) {
  const originalText = element.innerText;

  const textarea = document.createElement("textarea");
  textarea.value = originalText;
  textarea.style.width = "100%";
  textarea.style.resize = "vertical";
  textarea.style.height = element.scrollHeight + "px";
  element.replaceWith(textarea);

  // Disable dragging while editing so text can be selected
  const liContainer = textarea.closest(".todo-item");
  if (liContainer) {
    liContainer.setAttribute("draggable", "false");
  }

  textarea.dataset.id = Number(id);

  textarea.focus();
  textarea.select();

  // Handle Enter and Esc keys
  textarea.addEventListener("keypress", async function (e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      await saveEdit(textarea, originalText);
    } else if (e.key === "Escape") {
      e.preventDefault();
      const pre = document.createElement("pre");
      pre.className = "todo-text";
      pre.innerHTML = linkify(hashtagify(originalText));
      pre.addEventListener("click", function (e) {
        if (e.target.closest("a")) return;
        editTodo(id, pre);
      });
      textarea.replaceWith(pre);
      const liDone = pre.closest(".todo-item");
      if (liDone) liDone.setAttribute("draggable", "true");
    }
  });

  // Handle blur without saving
  textarea.addEventListener("blur", async function () {
    const pre = document.createElement("pre");
    pre.className = "todo-text";
    pre.innerHTML = linkify(hashtagify(originalText));
    pre.addEventListener("click", function (e) {
      if (e.target.closest("a")) return;
      editTodo(id, pre);
    });
    textarea.replaceWith(pre);
    const liDone = pre.closest(".todo-item");
    if (liDone) liDone.setAttribute("draggable", "true");
  });
}

async function saveEdit(textarea, originalText) {
  const id = Number(textarea.dataset.id);
  const newTitle = textarea.value.trim();
  if (newTitle === originalText.trim() || newTitle === "") {
    // Restore original text if no changes or empty
    const pre = document.createElement("pre");
    pre.className = "todo-text";
    pre.innerHTML = linkify(hashtagify(originalText));
    pre.addEventListener("click", function (e) {
      if (e.target.closest("a")) return;
      editTodo(id, pre);
    });
    textarea.replaceWith(pre);
    const liDone = pre.closest(".todo-item");
    if (liDone) liDone.setAttribute("draggable", "true");
    return;
  }

  try {
    const li = textarea.closest(".todo-item");
    const checkbox = li ? li.querySelector(".todo-checkbox") : null;
    const completed = checkbox ? checkbox.checked : false;
    const dueInput = li ? li.querySelector(".todo-date-input") : null;
    const timeInput = li ? li.querySelector(".todo-time-input") : null;
    let dueDate = null;

    // If date is explicitly cleared, clear the entire due date
    if (dueInput && dueInput.value === "") {
      dueDate = ""; // Send empty string to clear the date
    }
    // If time is set but no date, use today's date
    else if (timeInput?.value && (!dueInput || !dueInput.value)) {
      const now = new Date();
      const [hours, minutes] = timeInput.value.split(":").map(Number);
      // Create date in local timezone
      const localDate = new Date(
        now.getFullYear(),
        now.getMonth(),
        now.getDate(),
        hours,
        minutes,
        0,
        0,
      );
      // Convert to UTC and format as strict RFC3339 (no ms)
      dueDate = toRFC3339NoMillis(localDate);
    }
    // If date is set, use it with optional time
    else if (dueInput?.value) {
      const dateParts = dueInput.value.split("-");
      // Create date in local timezone
      const localDate = new Date(
        parseInt(dateParts[0]),
        parseInt(dateParts[1]) - 1, // months are 0-indexed
        parseInt(dateParts[2]),
      );

      if (timeInput?.value) {
        const [hours, minutes] = timeInput.value.split(":").map(Number);
        localDate.setHours(hours, minutes, 0, 0);
      } else {
        localDate.setHours(0, 0, 0, 0);
      }

      // Convert to UTC and format as strict RFC3339 (no ms)
      dueDate = toRFC3339NoMillis(localDate);
    }
    // Only use the existing due date if we're not explicitly clearing the date
    else if (dueInput?.dataset.due && !dueInput.value) {
      dueDate = dueInput.dataset.due;
      // If it's not already in ISO format, parse and convert it
      if (dueDate && !dueDate.endsWith("Z")) {
        const date = new Date(dueDate);
        if (!isNaN(date.getTime())) {
          dueDate = toRFC3339NoMillis(date);
        }
      }
      // Ensure dueDate is in RFC3339 format (no milliseconds)
      if (dueDate && dueDate.includes(".")) {
        dueDate = dueDate.replace(/\.\d{3}Z$/, "Z");
      }
    }
    const countEl = li ? li.querySelector(".recurrence-count") : null;
    const unitEl = li ? li.querySelector(".recurrence-unit") : null;

    // Log outgoing payload and due_date value/type
    const outgoingPayload = {
      id: Number(id),
      title: newTitle,
      completed: completed,
      due_date: dueDate,
      recurrence_interval:
        countEl && countEl.value ? Number(countEl.value) : null,
      recurrence_unit: unitEl && unitEl.value ? unitEl.value : null,
      position: Number(li.dataset.position),
    };

    const response = await fetch("/api/todo", {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(outgoingPayload),
    });

    if (!response.ok) {
      console.error("Response:", await response.json());
      throw new Error("Failed to update todo");
    }

    // Replace input with new pre
    const pre = document.createElement("pre");
    pre.className = "todo-text";
    pre.innerHTML = linkify(hashtagify(newTitle));
    pre.addEventListener("click", function (e) {
      if (e.target.closest("a")) return;
      editTodo(id, pre);
    });
    textarea.replaceWith(pre);
    const liDone = pre.closest(".todo-item");
    if (liDone) liDone.setAttribute("draggable", "true");
  } catch (error) {
    console.error("Error updating todo:", error);
    alert("Failed to update todo. Please try again.");
    // Restore original text
    const pre = document.createElement("pre");
    pre.className = "todo-text";
    pre.innerHTML = linkify(hashtagify(originalText));
    pre.addEventListener("click", function (e) {
      if (e.target.closest("a")) return;
      editTodo(id, pre);
    });
    textarea.replaceWith(pre);
    const liDone = pre.closest(".todo-item");
    if (liDone) liDone.setAttribute("draggable", "true");
  }
}

async function toggleTodo(id) {
  const checkbox = event.target;
  const todoItem = checkbox.parentElement;
  const titleElement = todoItem.querySelector(".todo-text");
  const title = titleElement.textContent;

  const dueInput = todoItem.querySelector(".todo-date-input");
  const timeInput = todoItem.querySelector(".todo-time-input");
  let dueDate = null;

  // If time is set but no date, use today's date
  if (timeInput?.value && (!dueInput || !dueInput.value)) {
    const now = new Date();
    const [hours, minutes] = timeInput.value.split(":").map(Number);
    // Create date in local timezone
    const localDate = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate(),
      hours,
      minutes,
      0,
      0,
    );
    // Convert to UTC and format as RFC3339 with 'Z' timezone
    dueDate = localDate.toISOString();
  }
  // If date is set, use it with optional time
  else if (dueInput?.value) {
    const dateParts = dueInput.value.split("-");
    // Create date in local timezone
    const localDate = new Date(
      parseInt(dateParts[0]),
      parseInt(dateParts[1]) - 1, // months are 0-indexed
      parseInt(dateParts[2]),
    );

    if (timeInput?.value) {
      const [hours, minutes] = timeInput.value.split(":").map(Number);
      localDate.setHours(hours, minutes, 0, 0);
    } else {
      localDate.setHours(0, 0, 0, 0);
    }

    // Convert to UTC and format as RFC3339 with 'Z' timezone
    dueDate = localDate.toISOString();
  }
  // Keep the existing due date if no new date is selected
  else if (dueInput?.dataset.due) {
    dueDate = dueInput.dataset.due;
  }

  const countEl = todoItem.querySelector(".recurrence-count");
  const unitEl = todoItem.querySelector(".recurrence-unit");
  const recInt = countEl && countEl.value ? Number(countEl.value) : null;
  const recUnit = unitEl && unitEl.value ? unitEl.value : null;

  try {
    const response = await fetch("/api/todo", {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        id: id,
        title: title,
        completed: checkbox.checked,
        due_date: dueDate,
        recurrence_interval: recInt,
        recurrence_unit: recUnit,
      }),
    });

    if (!response.ok) {
      throw new Error("Failed to toggle todo");
    }

    await loadTodosByProject();
  } catch (error) {
    console.error("Error toggling todo:", error);
    alert("Failed to toggle todo. Please try again.");
  }
}

async function deleteTodo(id) {
  try {
    const response = await fetch(`/api/todo?id=${id}`, {
      method: "DELETE",
    });

    if (!response.ok) {
      throw new Error("Failed to delete todo");
    }

    await loadTodosByProject();
  } catch (error) {
    console.error("Error deleting todo:", error);
    alert("Failed to delete todo. Please try again.");
  }
}

// Dark mode toggle
document
  .getElementById("darkModeToggle")
  .addEventListener("click", function () {
    document.body.classList.toggle("dark-mode");
    this.classList.toggle("dark-mode");

    // Save preference to localStorage
    if (document.body.classList.contains("dark-mode")) {
      localStorage.setItem("darkMode", "true");
    } else {
      localStorage.setItem("darkMode", "false");
    }
  });

