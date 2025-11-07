jQuery(function($){
    // Fetch posts for multiple types via REST
    async function fetchPostsForTypes(types) {
        const all = [];
        for (const t of types) {
            try {
                const posts = await $.getJSON(`${NakamaUIConfig.rest_base}/${t}?per_page=100`);
                posts.forEach(p => {
                    all.push({
                        id: p.id,
                        title: p.title && p.title.rendered ? p.title.rendered : `#${p.id}`,
                        type: t
                    });
                });
            } catch(e) {
                console.warn('Failed to fetch posts for type', t);
            }
        }
        return all;
    }

    function initMultiSelectPicker(container) {
        const $container = $(container);
        if ($container.data('initialized')) return;
        $container.data('initialized', true);
        const metaKey = $container.data('meta-key');
        const allowedTypes = $container.data('types').split(',');
        let options = [];
        let selectedIds = String($container.data('meta-value') || '')
            .split(',')
            .map(v => parseInt(v, 10))
            .filter(v => !isNaN(v) && v > 0);


        const $list = $('<div class="nak-picker-list" style="border:1px solid #ddd; padding:6px; min-height:120px;"></div>');
        const $hidden = $('<input type="hidden" name="'+metaKey+'" />').val(selectedIds.join(','));
        const $noItems = $('<p style="font-style:italic;color:#666; margin:0;"></p>');

        $container.append($list, $noItems, $hidden);

        function refreshList() {
            $list.empty();
            if (options.length === 0) {
                $noItems.text('No assets to select...');
                return;
            } else {
                $noItems.text('');
            }

            options.forEach(o => {
                const isSelected = selectedIds.includes(o.id);
                const $item = $('<div style="padding:4px; cursor:pointer; border-bottom:1px solid #eee;"></div>');
                $item.text(`${o.title} (${o.type})`);
                if (isSelected) $item.css({ background: '#dceefc' });
                
                $item.on('click', function(){
                    if (selectedIds.includes(o.id)) {
                        selectedIds = selectedIds.filter(id => id !== o.id);
                        $item.css({ background: '' });
                    } else {
                        selectedIds.push(o.id);
                        $item.css({ background: '#dceefc' });
                    }
                    $hidden.val(selectedIds.join(','));
                });

                $list.append($item);
            });
        }

        // Fetch options from REST API
        fetchPostsForTypes(allowedTypes).then(list => {
            $noItems.text('Loading items...');
            options = list;
            options.sort((a, b) => a.title.localeCompare(b.title, 'en', { sensitivity: 'base' }));
            refreshList();
        });

        // Initial render
        refreshList();
    }

    // Initialize all multi-select pickers
    $('.nakama-multi-select').each(function(){
        initMultiSelectPicker(this);
    });
});
