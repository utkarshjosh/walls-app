// Runs before the page is first drawn, so the layout that needs script (the
// pinned hero) is in place from the first frame instead of switching after it.
document.documentElement.classList.add("js");
