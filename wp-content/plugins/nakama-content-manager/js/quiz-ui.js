jQuery(function($){
    // Handle correct answer selection -> update correct text field
    $(document).on('change', 'input[name="correct"]', function() {
        const selected = $(this).val();
        const textInput = $('input[name="alt' + selected + '"]');
        if (textInput.length) {
            $('input[name="answer"]').val(textInput.val());
        }
    });

    // Update answer field when the alternative text changes (if selected)
    $(document).on('input', 'input[name^="alt"]', function() {
        const label = $(this).attr('name').replace('alt','');
        if ($('input[name="correct"]:checked').val() === label) {
            $('input[name="answer"]').val($(this).val());
        }
    });
});
