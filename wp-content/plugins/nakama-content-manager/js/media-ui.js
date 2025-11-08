jQuery(function ($) {
    $(document).on("click", ".nak-media-btn", function (e) {
        e.preventDefault();
        const $btn = $(this);
        const target = $btn.data("target");
        const type = $btn.data("type");      // "image" or "3d"
        const $row = $btn.closest(".nak-row");
        const $input = $row.find(`input[name="${target}"]`);

        // Allowed MIME types for validation
        const allowed3D = [
            "model/gltf-binary",   // .glb
            "model/gltf+json",     // .gltf
            "application/octet-stream" // sometimes used by some servers
        ];

        // Setup media frame
        const frame = wp.media({
            title: type === "3d" ? "Select or Upload 3D Model" : "Select or Upload Image",
            multiple: false,
            button: { text: "Use this file" },
            library: {
                // For 2D images we filter; for 3D we show ALL
                type: type === "image" ? "image" : null
            }
        });

        // For 3D, override filtering to show *all* files
        frame.on("open", function () {
            if (type === "3d") {
                const library = frame.state().get("library");
                // Remove type filter entirely
                library.props.set({ type: null, uploadedTo: null });
                library.props.set({ query: true });
                library.reset();
                library.fetch();
            }
        });

        // On selecting a file
        frame.on("select", function () {
            const file = frame.state().get("selection").first().toJSON();
            // Validation
            if (type === "image") {
                if (file.type !== "image") {
                    alert("Please select a valid image file.");
                    return;
                }
            }
            if (type === "3d") {
                if (!allowed3D.includes(file.mime)) {
                    alert("Please select a valid 3D model (.glb or .gltf).");
                    return;
                }
            }
            // Save ID
            $input.val(file.id);
            // Remove old previews
            $row.find(".nak-preview").remove();
            // Show preview
            if (file.type === "image") {
                $row.append(
                    $('<img class="nak-preview" style="max-width:200px; margin-top:6px;" />')
                        .attr("src", file.url)
                );
            } else {
                $row.append(
                    $(`<div class="nak-preview" style="margin-top:6px; font-style:italic;">File: ${file.filename}</div>`)
                );
            }
        });
        frame.open();
    });
});
