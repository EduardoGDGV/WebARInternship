( function( wp ) {
    const { registerPlugin } = wp.plugins;
    const { PluginDocumentSettingPanel } = wp.editor;
    const { Button, SelectControl, Spinner } = wp.components;
    const { useState, useEffect } = wp.element;
    const apiFetch = wp.apiFetch;
    const { useSelect, useDispatch } = wp.data;

    // Helper: fetch posts for multiple types (returns flattened array)
    async function fetchPostsForTypes(types) {
        // fetch first page (up to 100) per type and combine
        const all = [];
        for (const t of types) {
            try {
                const posts = await apiFetch({ path: `${NakamaUIConfig.rest_base}/${t}?per_page=100` });
                posts.forEach(p => {
                    all.push({ id: p.id, title: p.title && p.title.rendered ? p.title.rendered : `#${p.id}`, type: t });
                });
            } catch (e) {
                // ignore single-type failure
            }
        }
        // sort by title for nicer UI
        all.sort((a,b) => (a.title || '').localeCompare(b.title || ''));
        return all;
    }

    function UnifiedPicker({ metaKey, allowedTypes, label }) {
        const metaValue = useSelect( select => select( 'core/editor' ).getEditedPostAttribute( 'meta' )[ metaKey ] || [] );
        const { editPost } = useDispatch( 'core/editor' );

        const [loading, setLoading] = useState(true);
        const [options, setOptions] = useState([]);
        const [selected, setSelected] = useState('');

        useEffect(() => {
            let mounted = true;
            setLoading(true);
            fetchPostsForTypes(allowedTypes).then(list => {
                if (!mounted) return;
                setOptions(list);
                setLoading(false);
            });
            return () => { mounted = false; };
        }, []);

        function addSelected() {
            const id = parseInt(selected, 10);
            if (!id) return;
            if ((metaValue || []).includes(id)) return;
            const next = [ ...(metaValue || []), id ];
            editPost({ meta: { [metaKey]: next } });
            setSelected('');
        }

        function removeId(id) {
            const next = (metaValue || []).filter(v => v !== id);
            editPost({ meta: { [metaKey]: next } });
        }

        return wp.element.createElement('div', { style: { padding: '8px 0' } },
            wp.element.createElement('label', { style: { display: 'block', marginBottom: 6, fontWeight: 600 } }, label),
            loading ? wp.element.createElement(Spinner, {}) : wp.element.createElement(SelectControl, {
                value: selected,
                onChange: (val) => setSelected(val),
                options: [ { label: '— choose —', value: '' }, ...options.map(o => ({ label: `${o.title} (${o.type})`, value: String(o.id) })) ]
            }),
            wp.element.createElement(Button, { isPrimary: true, onClick: addSelected, disabled: !selected, style: { marginTop: 8 } }, 'Add'),
            wp.element.createElement('div', { style: { marginTop: 10 } },
                (metaValue || []).length === 0 ? wp.element.createElement('div', null, 'No items added') :
                (metaValue || []).map(id => {
                    const info = options.find(o => o.id === id);
                    const labelText = info ? `${info.title} (${info.type})` : `#${id}`;
                    return wp.element.createElement('div', { key: id, style: { display: 'flex', alignItems: 'center', gap: 8, marginTop: 6 } },
                        wp.element.createElement('span', null, labelText),
                        wp.element.createElement(Button, { isSecondary: true, onClick: () => removeId(id) }, 'Remove')
                    );
                })
            )
        );
    }

    function EventRelationsPanel() {
        const postType = useSelect( select => select( 'core/editor' ).getCurrentPostType() );
        if (postType !== 'event') return null;

        return wp.element.createElement(PluginDocumentSettingPanel, { name: 'nakama-event-relations', title: 'Event: Game Relations', className: 'nakama-event-relations' },
            wp.element.createElement(UnifiedPicker, { metaKey: 'requirements', allowedTypes: ['item','quiz'], label: NakamaUIConfig.labels.requirements }),
            wp.element.createElement(UnifiedPicker, { metaKey: 'rewards', allowedTypes: ['item','card'], label: NakamaUIConfig.labels.rewards })
        );
    }

    registerPlugin('nakama-event-relations', {
        render: EventRelationsPanel
    });

} )( window.wp );
