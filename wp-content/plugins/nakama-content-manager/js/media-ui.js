jQuery(function ($) {
    $(document).on("click", ".nak-media-btn", function (e) {
        e.preventDefault();

        const $btn = $(this);
        const target = $btn.data("target");
        const type = $btn.data("type");
        const $row = $btn.closest(".nak-row");
        const $input = $row.find(`input[name="${target}"]`);

        // Define allowed mime types
        const allowedTypes =
            type === "3d"
                ? ["model/gltf-binary", "model/gltf+json", "application/octet-stream"]
                : ["image"];

        // Create media frame (supports upload)
        const frame = wp.media({
            title: type === "3d" ? "Select or Upload 3D Model" : "Select or Upload Image",
            library: { type: allowedTypes },
            multiple: false,
            button: { text: "Use this file" },
            frame: "select",
        });

        // On file selection
        frame.on("select", function () {
            const file = frame.state().get("selection").first().toJSON();
            $input.val(file.id);

            // Remove any old preview or filename
            $row.find(".nak-preview").remove();

            // Create new preview depending on file type
            if (file.type === "image") {
                const img = $('<img class="nak-preview" style="max-width:200px; margin-top:6px;" />');
                img.attr("src", file.url);
                $row.append(img);
            } else {
                const fileInfo = $(
                    `<div class="nak-preview" style="margin-top:6px; font-style:italic;">File: ${file.filename}</div>`
                );
                $row.append(fileInfo);
            }
        });

        frame.open();
    });
});
