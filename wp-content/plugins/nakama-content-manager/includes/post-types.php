<?php
if (!defined('ABSPATH')) exit;

add_action('init', 'nakama_register_post_types');
function nakama_register_post_types() {

    // Event
    register_post_type('event', [
        'label' => 'Events',
        'public' => true,
        'show_ui' => true,
        'show_in_rest' => true,
        'supports' => ['title'],
        'menu_icon' => 'dashicons-format-image',
    ]);

    register_post_meta('event', 'lat', [
        'type' => 'number', 'single' => true, 'show_in_rest' => true,
    ]);
    register_post_meta('event', 'lon', [
        'type' => 'number', 'single' => true, 'show_in_rest' => true,
    ]);
    register_post_meta('event', 'image', [
        'type' => 'integer', 'single' => true, 'show_in_rest' => true,
    ]);
    register_post_meta('event', 'requirements', [
        'type' => 'array', 'single' => true,
        'show_in_rest' => ['schema' => ['type' => 'array', 'items' => ['type' => 'integer']]]
    ]);
    register_post_meta('event', 'rewards', [
        'type' => 'array', 'single' => true,
        'show_in_rest' => ['schema' => ['type' => 'array', 'items' => ['type' => 'integer']]]
    ]);

    // 2D Asset
    register_post_type('asset2d', [
        'label' => '2D Assets',
        'public' => true,
        'show_ui' => true,
        'show_in_rest' => true,
        'supports' => ['title'],
        'menu_icon' => 'dashicons-format-image',
    ]);
    register_post_meta('asset2d', 'image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);

    // Card
    register_post_type('card', [
        'label' => 'Cards',
        'public' => true,
        'show_ui' => true,
        'show_in_rest' => true,
        'supports' => ['title'],
        'menu_icon' => 'dashicons-format-image',
    ]);
    register_post_meta('card', 'front_image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('card', 'back_image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('card', 'group_card', ['type' => 'boolean', 'single' => true, 'default' => false, 'show_in_rest' => true]);

    // Item
    register_post_type('item', [
        'label' => 'Items',
        'public' => true,
        'show_ui' => true,
        'show_in_rest' => true,
        'supports' => ['title'],
        'menu_icon' => 'dashicons-format-image',
    ]);
    register_post_meta('item', 'image_2d', ['type' => 'integer', 'single' => true,
        'show_in_rest' => [
            'schema' => ['type' => 'string'],
            'get_callback' => function ($object) {
            $id = get_post_meta($object['id'], 'image_2d', true);
            return $id ? wp_get_attachment_url($id) : null;
            },
        ],
    ]);
    register_post_meta('item', 'image_3d', ['type' => 'integer', 'single' => true,
        'show_in_rest' => [
            'schema' => ['type' => 'string'],
            'get_callback' => function ($object) {
            $id = get_post_meta($object['id'], 'image_3d', true);
            return $id ? wp_get_attachment_url($id) : null;
            },
        ],
    ]);

    // Quiz
    register_post_type('quiz', [
        'label' => 'Quizzes',
        'public' => true,
        'show_ui' => true,
        'show_in_rest' => true,
        'supports' => ['title'],
        'menu_icon' => 'dashicons-format-image',
    ]);
    register_post_meta('quiz', 'question', ['type' => 'string', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('quiz', 'alternatives', [
        'type' => 'array', 'single' => true,
        'show_in_rest' => [
            'schema' => [
                'type' => 'array',
                'items' => ['type' => 'string'],
            ]
        ]
    ]);
    register_post_meta('quiz', 'answer', ['type' => 'string', 'single' => true, 'show_in_rest' => true]);
}


/**
 * Add a single Meta Box UI for each type
 */
add_action('add_meta_boxes', function() {
    $types = ['event', 'card', 'item', 'quiz', 'asset2d'];
    foreach ($types as $type) {
        add_meta_box('nakama_meta', 'Nakama Settings', 'nakama_render_meta_box', $type, 'normal', 'default');
    }
});


/**
 * Meta Box Renderer
 */
function nakama_render_meta_box($post) {
    wp_nonce_field('nakama_meta_save', 'nakama_meta_nonce');

    $type = get_post_type($post);

    // All selectable related posts (card, item, quiz)
    $selectable = get_posts([
        'post_type' => ['card', 'item', 'quiz'],
        'numberposts' => -1,
        'orderby' => 'title',
        'order' => 'ASC'
    ]);

    ?>
    <style>
        .nak-row { margin-bottom: 12px; }
        .nak-label { font-weight:bold; display:block; margin-bottom:4px; }
    </style>
    <?php

    if ($type === 'event') {
        nak_input('lat', $post);
        nak_input('lon', $post);
        nak_media('image', $post);

        $reqs = (array) get_post_meta($post->ID, 'requirements', true);
        $rewards = (array) get_post_meta($post->ID, 'rewards', true);
        ?>
            <div class="nak-row">
                <label class="nak-label">Requirements</label>
                <div class="nakama-multi-select"
                    data-meta-key="requirements"
                    data-types="quiz,event,item"
                    data-meta-value="<?php echo esc_attr(implode(',', array_map('intval', $reqs))); ?>">
                </div>
            </div>

            <div class="nak-row">
                <label class="nak-label">Rewards</label>
                <div class="nakama-multi-select"
                    data-meta-key="rewards"
                    data-types="item"
                    data-meta-value="<?php echo esc_attr(implode(',', array_map('intval', $rewards))); ?>">
                </div>
            </div>
        <?php
    }

    if ($type === 'card') {
        nak_media('front_image', $post);
        nak_media('back_image', $post);
        nak_checkbox('group_card', $post);
    }

    if ($type === 'item') {
        nak_media('image_2d', $post, '2D Image');
        nak_media('image_3d', $post, '3D Asset');
    }

    if ($type === 'quiz') {
        nak_textarea('question', $post);

        $alts = get_post_meta($post->ID, 'alternatives', true) ?: ['', '', '', ''];
        $letters = ['A', 'B', 'C', 'D'];
        $saved_answer = get_post_meta($post->ID, 'answer', true);

        echo "<div class='nak-row'><label class='nak-label'>Alternatives</label>";
        foreach ($letters as $i => $label) {
            $value = esc_attr($alts[$i] ?? '');
            $checked = checked($saved_answer, $label, false);
            echo "
            <div style='margin-bottom:6px;'>
                <label><strong>{$label}:</strong></label>
                <input type='text' name='alt{$label}' value='{$value}' style='width:60%;' />
                <label style='margin-left:10px;'>
                    <input type='radio' name='correct' value='{$label}' {$checked}> Correct
                </label>
            </div>";
        }
        echo "</div>";

        // Read-only display of correct answer
        echo "<div class='nak-row'>
                <label class='nak-label'>Correct Answer</label>
                <input type='text' name='answer' value='".esc_attr($saved_answer)."' class='widefat' readonly />
            </div>";
    }

    if ($type === 'asset2d') {
        nak_media('image', $post);
    }
}


/**
 * Field helpers
 */
function nak_input($key, $post, $label = null) {
    $label = $label ?: ucfirst($key);
    $val = esc_attr(get_post_meta($post->ID, $key, true));
    $id  = esc_attr($post->post_type . '_' . $key);

    echo "<div class='nak-row'>
            <label class='nak-label' for='{$id}'>{$label}</label>
            <input id='{$id}' type='text' name='{$key}' value='{$val}' class='widefat' />
          </div>";
}

function nak_checkbox($key, $post) {
    $val = get_post_meta($post->ID, $key, true);
    echo "<div class='nak-row'><label><input type='checkbox' name='{$key}' value='1' ".checked($val,true,false)." /> {$key}</label></div>";
}

function nak_textarea($key, $post) {
    $val = esc_textarea(get_post_meta($post->ID, $key, true));
    echo "<div class='nak-row'><label class='nak-label'>{$key}</label><textarea name='{$key}' rows='3' class='widefat'>{$val}</textarea></div>";
}

function nak_multi_select($key, $post, $posts) {
    $selected = get_post_meta($post->ID, $key, true) ?: [];
    echo "<div class='nak-row'><label class='nak-label'>{$key}</label><select name='{$key}[]' multiple class='widefat' style='min-height:120px;'>";
    foreach ($posts as $p) {
        $sel = selected(in_array($p->ID, $selected), true, false);
        echo "<option value='{$p->ID}' {$sel}>{$p->post_title} ({$p->post_type})</option>";
    }
    echo "</select></div>";
}

function nak_media($key, $post, $type = 'image') {
    $id_or_url = get_post_meta($post->ID, $key, true);
    $attachment_url = $id_or_url ? wp_get_attachment_url($id_or_url) : '';

    // Allowed formats
    $img_ext = ['png', 'jpg', 'jpeg', 'gif', 'webp'];
    $model_ext = ['glb', 'gltf'];

    $ext = $attachment_url ? strtolower(pathinfo($attachment_url, PATHINFO_EXTENSION)) : '';

    // Only show preview if image
    $can_preview = in_array($ext, $img_ext);

    ?>
    <div class="nak-row">
        <?php $label = ucwords(str_replace(['_', '-'], ' ', $key)); ?>
        <label class="nak-label"><?= esc_html($label) ?></label>

        <?php $id = esc_attr($post->post_type . '_' . $key); ?>
        <input id="<?= $id ?>" type="hidden" name="<?= $key ?>" value="<?= esc_attr($id_or_url) ?>" />

        <button type="button" class="button nak-media-btn"
                data-target="<?= $key ?>"
                data-type="<?= esc_attr($type) ?>">
            Select File
        </button>

        <?php if ($attachment_url): ?>
            <?php if ($can_preview): ?>
                <div><img src="<?= esc_url($attachment_url) ?>" style="max-width:200px; margin-top:6px;" /></div>
            <?php else: ?>
                <div style="margin-top:6px; font-style:italic;">
                    File: <?= basename($attachment_url) ?>
                </div>
            <?php endif; ?>
        <?php endif; ?>

        <?php if ($type === '3d'): ?>
            <p style="font-size:11px; color:#555;">
                Allowed: *.glb, *.gltf
            </p>
        <?php endif; ?>
    </div>
    <?php
}


/**
 * Save callback
 */
add_action('save_post', function($post_id) {
    if (!isset($_POST['nakama_meta_nonce']) ||
        !wp_verify_nonce($_POST['nakama_meta_nonce'], 'nakama_meta_save')) return;
    if (defined('DOING_AUTOSAVE') && DOING_AUTOSAVE) return;

    $type = get_post_type($post_id);

    if ($type === 'quiz') {
        $letters = ['A', 'B', 'C', 'D'];
        $alts = [];
        foreach ($letters as $label) {
            if(isset($_POST['alt'.$label])) {
                $alts[] = sanitize_text_field($_POST['alt'.$label]);
            }
        }
        update_post_meta($post_id, 'alternatives', $alts);

        if(isset($_POST['correct'])) {
            update_post_meta($post_id, 'answer', sanitize_text_field($_POST['correct']));
        }

        if(isset($_POST['question'])) {
            update_post_meta($post_id, 'question', sanitize_text_field($_POST['question']));
        }

        return; // Stop here for quiz, don't update unrelated fields
    }

    $fields = ['lat','lon','image','front_image','back_image',
        'image_2d','image_3d','question','answer'];

    foreach ($fields as $f) {
        if (isset($_POST[$f])) update_post_meta($post_id,$f,sanitize_text_field($_POST[$f]));
    }

    // Boolean
    update_post_meta($post_id,'group_card', isset($_POST['group_card']));

    // Multi selects
    foreach (['requirements', 'rewards'] as $arr) {
        if (isset($_POST[$arr])) {
            // Convert comma-separated string into array of ints
            $vals = is_array($_POST[$arr]) ? $_POST[$arr] : explode(',', $_POST[$arr]);
            update_post_meta($post_id, $arr, array_map('intval', $vals));
        } else {
            delete_post_meta($post_id, $arr);
        }
    }
});

add_filter('upload_mimes', function ($mimes) {
    $mimes['glb']  = 'model/gltf-binary';
    $mimes['gltf'] = 'model/gltf+json';
    return $mimes;
});

// Register how media appears in REST (URLs)
add_action('rest_api_init', function () {
    //Exposing event fields
    register_rest_field('event', 'requirements', [
        'get_callback' => function ($object) {
            $ids = get_post_meta($object['id'], 'requirements', true);
            return is_array($ids) ? $ids : array_filter(array_map('intval', explode(',', (string)$ids)));
        },
        'schema' => [
            'description' => 'Requirement IDs',
            'type'        => 'array',
            'items'       => ['type' => 'integer'],
        ],
    ]);

    register_rest_field('event', 'rewards', [
        'get_callback' => function ($object) {
            $ids = get_post_meta($object['id'], 'rewards', true);
            return is_array($ids) ? $ids : array_filter(array_map('intval', explode(',', (string)$ids)));
        },
        'schema' => [
            'description' => 'Reward IDs',
            'type'        => 'array',
            'items'       => ['type' => 'integer'],
        ],
    ]);

    foreach (['lat', 'lon'] as $key) {
        register_rest_field('event', $key, [
            'get_callback' => fn($object) => get_post_meta($object['id'], $key, true),
            'schema' => [
                'description' => ucfirst($key),
                'type'        => 'number',
            ],
        ]);
    }

    // Exposing quiz fields
    register_rest_field('quiz', 'question', [
        'get_callback' => function ($object) {
            return get_post_meta($object['id'], 'question', true) ?: null;
        },
        'schema' => [
            'description' => 'Quiz question',
            'type'        => 'string',
            'context'     => ['view', 'edit'],
        ],
    ]);

    register_rest_field('quiz', 'alternatives', [
        'get_callback' => function ($object) {
            $alts = get_post_meta($object['id'], 'alternatives', true);
            if (empty($alts)) return [];
            return (array) $alts;
        },
        'schema' => [
            'description' => 'Quiz alternatives (A..D)',
            'type'        => 'array',
            'items'       => ['type' => 'string'],
            'context'     => ['view', 'edit'],
        ],
    ]);

    register_rest_field('quiz', 'answer', [
        'get_callback' => function ($object) {
            return get_post_meta($object['id'], 'answer', true) ?: null;
        },
        'schema' => [
            'description' => 'Correct answer label (A|B|C|D)',
            'type'        => 'string',
            'context'     => ['view', 'edit'],
        ],
    ]);

    // Helper to register one or more image fields for a post type
    function register_image_fields($post_type, $fields) {
        foreach ($fields as $meta_key => $label) {
            register_rest_field($post_type, $label, [
                'get_callback' => function ($object) use ($meta_key) {
                    $id = get_post_meta($object['id'], $meta_key, true);
                    return $id ? wp_get_attachment_url($id) : null;
                },
                'schema' => [
                    'description' => ucfirst($label) . ' image or model URL',
                    'type'        => 'string',
                    'context'     => ['view', 'edit'],
                ],
            ]);
        }
    }
    // Item post type 2D + 3D
    register_image_fields('item', [
        'image_2d' => '2d',
        'image_3d' => '3d',
    ]);
    // Card post type front + back
    register_image_fields('card', [
        'front_image' => 'front',
        'back_image'  => 'back',
    ]);
    // Asset2D post type single image
    register_image_fields('asset2d', [
        'image' => 'image',
    ]);
    // Event post type single image
    register_image_fields('event', [
        'image' => 'image',
    ]);
});
