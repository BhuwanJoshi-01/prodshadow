// Copy buttons outside code blocks (the landing page's install command).
// Delegated on the document so it keeps working with instant navigation.
document.addEventListener("click", function (event) {
  var button = event.target.closest("[data-ps-copy]");
  if (!button || !navigator.clipboard) {
    return;
  }
  navigator.clipboard.writeText(button.getAttribute("data-ps-copy")).then(function () {
    button.classList.add("is-copied");
    button.setAttribute("title", "Copied");
    window.setTimeout(function () {
      button.classList.remove("is-copied");
      button.setAttribute("title", "Copy to clipboard");
    }, 1600);
  });
});
