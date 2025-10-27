jQuery(function($){
    $('.nak-media-btn').click(function(e){
        e.preventDefault();

        const target = $(this).data('target');
        const type = $(this).data('type');

        // Create the media frame.
        const frame = wp.media({
            title: 'Select or Upload File',
            library: {
                type: type === '3d'
                    ? ['model/gltf-binary', 'model/gltf+json']
                    : 'image'
            },
            multiple: false,           // Only allow single selection
            button: { text: 'Use this file' }, // Button text
            frame: 'select'             // Default frame type allows upload tab
        });

        // When a file is selected, update the input and preview.
        frame.on('select', function(){
            const file = frame.state().get('selection').first().toJSON();
            $('input[name="' + target + '"]').val(file.id);

            const wrapper = $('input[name="' + target + '"]').closest('.nak-row');

            if (file.type === 'image') {
                wrapper.find('img').attr('src', file.url).show();
            } else {
                wrapper.find('img').hide();
            }
        });

        // Open the media frame.
        frame.open();
    });
});
